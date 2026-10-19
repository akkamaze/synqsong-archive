package auth

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"
)

var t0 = time.Unix(1_790_000_000, 0)

func ip(raw string) ClientAddr {
	a, err := NewClientAddr(netip.MustParseAddr(raw))
	if err != nil {
		panic(err)
	}

	return a
}

var (
	ann       = usernameKey([]byte("ann"))
	bob       = usernameKey([]byte("bob"))
	annLaptop = signInAddrKey(ip("10.0.0.1"))
)

// fail reserves a try on every key and records it as failed, or returns why it was refused.
func fail(l *Limiter, at time.Time, keys ...limitKey) error {
	attempt, err := l.Reserve(at, keys...)
	if err != nil {
		return err
	}

	attempt.Fail(at)

	return nil
}

func succeed(l *Limiter, at time.Time, keys ...limitKey) error {
	attempt, err := l.Reserve(at, keys...)
	if err != nil {
		return err
	}

	attempt.Succeed(at)

	return nil
}

func lockedFor(err error) time.Duration {
	var locked *LockedError
	if !errors.As(err, &locked) {
		return 0
	}

	return locked.RetryAfter
}

func TestFiveFailuresLockAUsernameForTenSeconds(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)

	for i := range 5 {
		if err := fail(l, t0, ann); err != nil {
			t.Fatalf("attempt %d refused: %v, want the first 5 allowed", i+1, err)
		}
	}

	if err := fail(l, t0, ann); lockedFor(err) != 10*time.Second {
		t.Fatalf("6th attempt: %v, want locked for 10s", err)
	}

	if err := fail(l, t0.Add(10*time.Second), ann); err != nil {
		t.Fatalf("attempt after the lock ended: %v, want allowed", err)
	}
}

func TestLocksGrowAndStopAtAnHour(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)
	at := t0
	want := []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, time.Hour}

	for round, lock := range want {
		for range 5 {
			if err := fail(l, at, ann); err != nil {
				t.Fatalf("round %d: attempt refused early: %v", round+1, err)
			}
		}

		if err := fail(l, at, ann); lockedFor(err) != lock {
			t.Fatalf("round %d lock = %v, want %v", round+1, err, lock)
		}

		at = at.Add(lock)
	}
}

func TestSuccessesFromOneAddressNeverLockIt(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)

	for i := range 100 {
		if err := succeed(l, t0, usernameKey([]byte{byte(i)}), annLaptop); err != nil {
			t.Fatalf("sign-in %d from one address refused: %v, want successes never counted", i+1, err)
		}
	}

	for i := range 20 {
		_ = fail(l, t0, usernameKey([]byte{byte(100 + i)}), annLaptop)
	}

	if err := succeed(l, t0, ann, annLaptop); lockedFor(err) == 0 {
		t.Fatalf("attempt after 20 failures from one address: %v, want the address locked", err)
	}
}

func TestASuccessStartsAUsernameOver(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)

	for range 4 {
		_ = fail(l, t0, ann)
	}

	_ = succeed(l, t0, ann)

	for i := range 5 {
		if err := fail(l, t0, ann); err != nil {
			t.Fatalf("attempt %d after a success refused: %v, want the count reset", i+1, err)
		}
	}
}

func TestStrikesAreForgottenAfterADayWithoutFailures(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)

	for range 6 {
		_ = fail(l, t0, ann)
	}

	later := t0.Add(25 * time.Hour)
	for range 5 {
		_ = fail(l, later, ann)
	}

	if err := fail(l, later, ann); lockedFor(err) != 10*time.Second {
		t.Fatalf("lock a day later = %v, want back to the first 10s step", err)
	}
}

func TestAnyLockedKeyRefusesTheAttemptWithoutReservingTheOthers(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)

	for range 6 {
		_ = fail(l, t0, bob)
	}

	if err := fail(l, t0, ann, bob); lockedFor(err) == 0 {
		t.Fatalf("attempt with one locked key: %v, want locked", err)
	}

	if err := fail(l, t0, bob, ann); lockedFor(err) == 0 {
		t.Fatalf("attempt with the locked key first: %v, want locked", err)
	}

	for i := range 5 {
		if err := fail(l, t0, ann); err != nil {
			t.Fatalf("fresh key attempt %d refused: %v, want the refused attempts not counted", i+1, err)
		}
	}
}

func TestTriesInFlightCountTowardTheLimit(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)
	inFlight := make([]*Attempt, 0, 5)

	for i := range 5 {
		attempt, err := l.Reserve(t0, ann)
		if err != nil {
			t.Fatalf("reservation %d refused: %v", i+1, err)
		}

		inFlight = append(inFlight, attempt)
	}

	if _, err := l.Reserve(t0, ann); !errors.Is(err, ErrBusy) {
		t.Fatalf("6th try while 5 are in flight: %v, want ErrBusy", err)
	}

	inFlight[0].Release()

	if _, err := l.Reserve(t0, ann); err != nil {
		t.Fatalf("try after one in-flight try was released: %v, want allowed", err)
	}
}

func TestAReleasedTryIsNotCounted(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)

	for range 50 {
		attempt, err := l.Reserve(t0, ann)
		if err != nil {
			t.Fatalf("reservation refused: %v, want released tries forgotten", err)
		}

		attempt.Release()
	}
}

func TestAnAttemptEndsOnce(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)

	for range 5 {
		attempt, _ := l.Reserve(t0, ann)
		attempt.Succeed(t0)
		attempt.Fail(t0)
		attempt.Release()
	}

	if err := fail(l, t0, ann); err != nil {
		t.Fatalf("try after 5 successes: %v, want the later Fail calls ignored", err)
	}
}

func size(l *Limiter) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.keys)
}

func TestKeysWithNothingLeftToRememberArePruned(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)

	for i := range 100 {
		_ = fail(l, t0, usernameKey([]byte{byte(i)}))
	}

	for range 6 {
		_ = fail(l, t0, bob)
	}

	held, _ := l.Reserve(t0, ann)
	later := t0.Add(25 * time.Hour)

	for range 6 {
		_ = fail(l, later, annLaptop)
	}

	for range pruneEvery {
		attempt, _ := l.Reserve(later, usernameKey([]byte("someone else")))
		attempt.Release()
	}

	if got := size(l); got != 2 {
		t.Fatalf("keys kept = %d, want 2: the try still in flight and the address that failed today", got)
	}

	held.Release()
}

func TestAFullLimiterDropsTheKeyLeastWorthKeeping(t *testing.T) {
	l := NewLimiter(3)

	for range 5 {
		_ = fail(l, t0, ann)
	}

	_ = fail(l, t0, bob)
	_ = fail(l, t0.Add(time.Second), annLaptop)

	if err := fail(l, t0.Add(2*time.Second), usernameKey([]byte("new"))); err != nil {
		t.Fatalf("new key on a full limiter: %v, want room made", err)
	}

	if got := size(l); got > 3 {
		t.Fatalf("keys = %d, want at most 3", got)
	}

	if err := fail(l, t0.Add(2*time.Second), ann); lockedFor(err) == 0 {
		t.Fatalf("locked key after the limiter was full: %v, want still locked", err)
	}

	for i := range 4 {
		if err := fail(l, t0.Add(3*time.Second), bob); err != nil {
			t.Fatalf("bob try %d: %v", i+1, err)
		}
	}

	if err := fail(l, t0.Add(3*time.Second), bob); err != nil {
		t.Fatalf("bob's 5th try since being dropped: %v, want his oldest failure forgotten", err)
	}
}

func TestALimiterFullOfLocksRefusesNewKeys(t *testing.T) {
	l := NewLimiter(2)

	for range 5 {
		_ = fail(l, t0, ann, bob)
	}

	if err := fail(l, t0, annLaptop); !errors.Is(err, ErrBusy) {
		t.Fatalf("new key while every kept key is locked: %v, want ErrBusy", err)
	}

	if err := fail(l, t0, bob); lockedFor(err) == 0 {
		t.Fatalf("locked key after a refused new key: %v, want still locked", err)
	}
}

func TestAFullLimiterMakesRoomForEveryNewKeyOfATry(t *testing.T) {
	l := NewLimiter(2)

	_ = fail(l, t0, bob)

	if err := fail(l, t0.Add(time.Second), ann, annLaptop); err != nil {
		t.Fatalf("two new keys with room for one: %v, want room made", err)
	}

	if got := size(l); got != 2 {
		t.Fatalf("keys = %d, want 2", got)
	}
}

func TestAFullLimiterNeverDropsAKeyWithATryInFlight(t *testing.T) {
	l := NewLimiter(2)
	held, _ := l.Reserve(t0, ann)
	_ = fail(l, t0, bob)

	for i := range 3 {
		_ = fail(l, t0, usernameKey([]byte{byte(i)}))
	}

	held.Fail(t0)

	for i := range 4 {
		if _, err := l.Reserve(t0, ann); err != nil {
			t.Fatalf("try %d after one failure: %v, want 4 more allowed in flight", i+1, err)
		}
	}

	if _, err := l.Reserve(t0, ann); !errors.Is(err, ErrBusy) {
		t.Fatalf("6th try counting the failure that was in flight: %v, want ErrBusy", err)
	}
}

func TestAFloodOfNewKeysKeepsTheLocksAKeyEarned(t *testing.T) {
	l := NewLimiter(10)
	at := t0

	for _, lock := range []time.Duration{10 * time.Second, time.Minute} {
		for range 5 {
			_ = fail(l, at, ann)
		}

		at = at.Add(lock)
	}

	for i := range 50 {
		if err := fail(l, at, usernameKey([]byte{byte(i)})); err != nil {
			t.Fatalf("flood key %d: %v, want room made", i+1, err)
		}
	}

	for range 5 {
		_ = fail(l, at, ann)
	}

	if err := fail(l, at, ann); lockedFor(err) != 5*time.Minute {
		t.Fatalf("third lock after a flood of new names = %v, want 5m: the flood must not reset earned locks", err)
	}

	if got := size(l); got > 10 {
		t.Fatalf("keys = %d, want at most 10", got)
	}
}

func BenchmarkANewKeyOnAFullLimiter(b *testing.B) {
	l := NewLimiter(limiterMaxKeys)

	for i := range limiterMaxKeys {
		_ = fail(l, t0, usernameKey(binary.BigEndian.AppendUint64(nil, uint64(i))))
	}

	at := t0.Add(time.Minute)
	next := uint64(limiterMaxKeys)

	for b.Loop() {
		next++
		if err := fail(l, at, usernameKey(binary.BigEndian.AppendUint64(nil, next))); err != nil {
			b.Fatalf("new key %d: %v", next, err)
		}
	}

	if got := size(l); got > limiterMaxKeys {
		b.Fatalf("keys = %d, want at most %d", got, limiterMaxKeys)
	}

	b.ReportMetric(float64(size(l)), "keys")
}

func TestSignUpsAreCountedPerDayNotPerRunOfTries(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)
	office := signUpAddrKey(ip("203.0.113.9"))

	for range 50 {
		_ = succeed(l, t0, office)
	}

	for range 5 {
		_ = succeed(l, t0.Add(23*time.Hour), office)
	}

	nextDay := t0.Add(24 * time.Hour)

	for i := range 59 {
		if err := succeed(l, nextDay, office); err != nil {
			t.Fatalf("sign-up %d on the next day: %v, want a new day's 60", i+1, err)
		}
	}

	if err := succeed(l, nextDay, office); err != nil {
		t.Fatalf("60th sign-up of the day: %v, want allowed", err)
	}

	if err := succeed(l, nextDay, office); lockedFor(err) != 24*time.Hour {
		t.Fatalf("61st sign-up of the day: %v, want locked for 24h", err)
	}
}

func TestClearingALoginLiftsItsLocksAndKeepsTriesInFlight(t *testing.T) {
	l := NewLimiter(limiterMaxKeys)
	laptop, phone := deviceKey([]byte("ann"), "laptop"), deviceKey([]byte("ann"), "phone")
	bobsPhone := deviceKey([]byte("bob"), "phone")

	for range 5 {
		_ = fail(l, t0, ann)
		_ = fail(l, t0, laptop)
		_ = fail(l, t0, bobsPhone)
	}

	held, _ := l.Reserve(t0, phone)
	l.ClearLogin([]byte("ann"))
	held.Fail(t0)

	for name, k := range map[string]limitKey{"username": ann, "device": laptop} {
		if err := fail(l, t0, k); err != nil {
			t.Fatalf("%s of a cleared login: %v, want the lock lifted", name, err)
		}
	}

	if err := fail(l, t0, bobsPhone); lockedFor(err) == 0 {
		t.Fatalf("another login's device: %v, want still locked", err)
	}

	for i := range 4 {
		if _, err := l.Reserve(t0, phone); err != nil {
			t.Fatalf("try %d after the failure that was in flight: %v", i+1, err)
		}
	}

	if _, err := l.Reserve(t0, phone); !errors.Is(err, ErrBusy) {
		t.Fatalf("6th try counting the failure that was in flight across the clear: %v, want ErrBusy", err)
	}
}

func TestReservingAForgottenKeyOnAFullLimiterStaysWithinTheCap(t *testing.T) {
	l := NewLimiter(2)
	_ = fail(l, t0, ann)

	later := t0.Add(usernamePolicy.Forget)
	_ = fail(l, later, bob)

	if _, err := l.Reserve(later, ann, usernameKey([]byte("carl"))); err != nil {
		t.Fatalf("reserve a forgotten key and a new one: %v", err)
	}

	if n := len(l.keys); n > 2 {
		t.Fatalf("limiter holds %d keys, want at most its cap of 2", n)
	}
}
