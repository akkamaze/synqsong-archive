package auth

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Policy says how many tries a key may have before it is locked, and for how long.
type Policy struct {
	Tries int
	// Locks grow one step per lock and stay at the last step.
	Locks []time.Duration
	// Forget drops a key's history this long after its last counted try.
	Forget time.Duration
	// Window, when set, starts the count over this long after its first try, so Tries is a
	// limit per window rather than per run of tries.
	Window time.Duration
	// OnSuccess is what a successful try does to the key's count.
	OnSuccess SuccessRule
}

// SuccessRule says what a successful try does to a key's count.
type SuccessRule int8

const (
	// StartOver forgets the key's failures and locks, as for a username that signed in.
	StartOver SuccessRule = iota + 1
	// NotCounted leaves the count as it was, so an address is never locked by its successes.
	NotCounted
	// Counted counts a success like a failure, as for sign-ups from one address.
	Counted
)

// A username locks for 10s after 5 failures; later locks grow so 4 digits cannot be walked.
var (
	growingLocks     = []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	usernamePolicy   = Policy{Tries: 5, Locks: growingLocks, Forget: 24 * time.Hour, OnSuccess: StartOver}
	devicePolicy     = Policy{Tries: 5, Locks: growingLocks, Forget: 24 * time.Hour, OnSuccess: StartOver}
	signInAddrPolicy = Policy{Tries: 20, Locks: growingLocks, Forget: 24 * time.Hour, OnSuccess: NotCounted}
	// 60 a day lets an office behind one public address sign everyone up on launch day.
	signUpAddrPolicy = Policy{Tries: 60, Locks: []time.Duration{24 * time.Hour}, Forget: 24 * time.Hour, Window: 24 * time.Hour, OnSuccess: Counted}
)

const (
	// limiterMaxKeys bounds memory at about 16 MB however many names or addresses are tried.
	limiterMaxKeys = 100_000
	// pruneEvery spreads the cost of dropping forgotten keys over many reservations.
	pruneEvery = 1024
	// A full limiter frees one in evictShare of its keys in one scan.
	evictShare = 100
)

// limitKey is one thing tries are counted against, such as a username or a client address.
type limitKey struct {
	policy *Policy
	id     string
}

func usernameKey(digest []byte) limitKey {
	return limitKey{policy: &usernamePolicy, id: string(digest)}
}

// deviceKey starts with the username digest, so an admin reset can find every device of a login.
func deviceKey(usernameDigest []byte, device string) limitKey {
	return limitKey{policy: &devicePolicy, id: string(usernameDigest) + device}
}

func signInAddrKey(a ClientAddr) limitKey {
	return limitKey{policy: &signInAddrPolicy, id: a.String()}
}

func signUpAddrKey(a ClientAddr) limitKey {
	return limitKey{policy: &signUpAddrPolicy, id: a.String()}
}

// LockedError says when to try again, so the sign-in screen can count down.
type LockedError struct {
	RetryAfter time.Duration
}

func (e *LockedError) Error() string {
	return fmt.Sprintf("too many tries, wait %s and try again", e.RetryAfter.Round(time.Second))
}

type tries struct {
	counted      int
	pending      int
	locks        int
	until        time.Time
	firstCounted time.Time
	lastCounted  time.Time
}

// Limiter counts tries per key in memory. It lives in one process.
type Limiter struct {
	mu       sync.Mutex
	keys     map[limitKey]*tries
	maxKeys  int
	reserves int
}

func NewLimiter(maxKeys int) *Limiter {
	return &Limiter{keys: map[limitKey]*tries{}, maxKeys: maxKeys}
}

// Attempt is a try reserved on some keys. It ends once, by Fail, Succeed or Release.
type Attempt struct {
	limiter *Limiter
	keys    []limitKey
	ended   bool
}

// Reserve refuses the try if any key is locked, or if its counted and in-flight tries reach
// the limit, so many requests sent at once get no more tries than one at a time.
func (l *Limiter) Reserve(now time.Time, keys ...limitKey) (*Attempt, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var wait time.Duration

	for _, k := range keys {
		if s, ok := l.keys[k]; ok && now.Before(s.until) {
			wait = max(wait, s.until.Sub(now))
		}
	}

	if wait > 0 {
		return nil, &LockedError{RetryAfter: wait}
	}

	for _, k := range keys {
		if s, ok := l.keys[k]; ok && s.countedAt(now, k.policy)+s.pending >= k.policy.Tries {
			return nil, ErrBusy
		}
	}

	l.reserves++
	if l.reserves%pruneEvery == 0 {
		l.prune(now)
	}

	if !l.makeRoom(now, keys) {
		return nil, ErrBusy
	}

	for _, k := range keys {
		s := l.entry(k)
		s.forgetIfStale(now, k.policy)
		s.pending++
	}

	return &Attempt{limiter: l, keys: keys}, nil
}

func (l *Limiter) prune(now time.Time) {
	for k, s := range l.keys {
		if s.forgotten(now, k.policy) {
			delete(l.keys, k)
		}
	}
}

// makeRoom keeps the map within maxKeys. When full, one scan frees a batch of keys, so the
// scan's cost is shared by many new keys. It refuses only when every kept key is locked or in use.
func (l *Limiter) makeRoom(now time.Time, keys []limitKey) bool {
	needed := 0

	for _, k := range keys {
		if _, ok := l.keys[k]; !ok {
			needed++
		}
	}

	if len(l.keys)+needed <= l.maxKeys {
		return true
	}

	l.evict(now, keys, max(needed, l.maxKeys/evictShare))

	return len(l.keys)+needed <= l.maxKeys
}

type evictable struct {
	key   limitKey
	tries *tries
}

// evict drops every forgotten key, then the idle unlocked keys least worth keeping until want
// are gone. A key with a try in flight or a live lock, or one of keep, is never dropped.
func (l *Limiter) evict(now time.Time, keep []limitKey, want int) {
	var candidates []evictable

	for k, s := range l.keys {
		switch {
		case slices.Contains(keep, k):
		case s.forgotten(now, k.policy):
			delete(l.keys, k)
			want--
		case s.pending == 0 && !now.Before(s.until):
			candidates = append(candidates, evictable{key: k, tries: s})
		}
	}

	if want <= 0 {
		return
	}

	slices.SortFunc(candidates, func(a, b evictable) int { return a.tries.worth(b.tries) })

	for _, c := range candidates[:min(want, len(candidates))] {
		delete(l.keys, c.key)
	}
}

func (l *Limiter) entry(k limitKey) *tries {
	s, ok := l.keys[k]
	if !ok {
		s = &tries{}
		l.keys[k] = s
	}

	return s
}

// ClearLogin forgets the failures and locks of a username and of every device that signed
// in to it, as when an admin gives someone a new PIN. Tries still in flight stay reserved.
func (l *Limiter) ClearLogin(usernameDigest []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()

	name := usernameKey(usernameDigest)

	for k, s := range l.keys {
		if k != name && (k.policy != &devicePolicy || !strings.HasPrefix(k.id, name.id)) {
			continue
		}

		*s = tries{pending: s.pending}

		if s.empty() {
			delete(l.keys, k)
		}
	}
}

// Fail counts the try on every key, which may lock them.
func (a *Attempt) Fail(now time.Time) {
	a.end(func(k limitKey, s *tries) { s.count(now, k.policy) })
}

// Succeed applies each key's success rule.
func (a *Attempt) Succeed(now time.Time) {
	a.end(func(k limitKey, s *tries) {
		switch k.policy.OnSuccess {
		case StartOver:
			*s = tries{pending: s.pending}
		case NotCounted:
		case Counted:
			s.count(now, k.policy)
		}
	})
}

// Release ends the try without counting it, as when no PIN was checked. Deferring it
// covers every early return; it does nothing once the attempt has ended.
func (a *Attempt) Release() {
	a.end(func(limitKey, *tries) {})
}

func (a *Attempt) end(apply func(limitKey, *tries)) {
	l := a.limiter
	l.mu.Lock()
	defer l.mu.Unlock()

	if a.ended {
		return
	}

	a.ended = true

	for _, k := range a.keys {
		s := l.entry(k)
		s.pending--
		apply(k, s)

		if s.empty() {
			delete(l.keys, k)
		}
	}
}

func (s *tries) countedAt(now time.Time, p *Policy) int {
	if now.Sub(s.lastCounted) >= p.Forget || s.windowEnded(now, p) {
		return 0
	}

	return s.counted
}

func (s *tries) windowEnded(now time.Time, p *Policy) bool {
	return p.Window > 0 && now.Sub(s.firstCounted) >= p.Window
}

func (s *tries) forgetIfStale(now time.Time, p *Policy) {
	switch {
	case now.Sub(s.lastCounted) >= p.Forget:
		*s = tries{pending: s.pending}
	case s.windowEnded(now, p):
		s.counted = 0
	}
}

func (s *tries) count(now time.Time, p *Policy) {
	s.forgetIfStale(now, p)

	if s.counted == 0 {
		s.firstCounted = now
	}

	s.counted++
	s.lastCounted = now

	if s.counted >= p.Tries {
		step := min(s.locks, len(p.Locks)-1)
		s.until = now.Add(p.Locks[step])
		s.locks++
		s.counted = 0
	}
}

func (s *tries) forgotten(now time.Time, p *Policy) bool {
	return s.pending == 0 && !now.Before(s.until) && now.Sub(s.lastCounted) >= p.Forget
}

// worth ranks keys for eviction, least worth first: dropping a key that earned more locks or
// more failures would give a guesser more tries, so those are kept longest.
func (s *tries) worth(o *tries) int {
	return cmp.Or(cmp.Compare(s.locks, o.locks), cmp.Compare(s.counted, o.counted), s.lastCounted.Compare(o.lastCounted))
}

func (s *tries) empty() bool {
	return s.pending == 0 && s.counted == 0 && s.locks == 0
}
