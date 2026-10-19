package auth

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"synqsong/internal/account"
)

type fakeSession struct {
	account  account.ID
	lastUsed time.Time
}

type fakeStore struct {
	mu       sync.Mutex
	now      func() time.Time
	next     account.ID
	byName   map[string]PINLogin
	roles    map[account.ID]account.SiteRole
	sessions map[string]fakeSession
	lookups  int
	// beforeSession runs between the PIN check and the session insert, where a PIN change can race.
	beforeSession func()
}

func newFakeStore(now func() time.Time) *fakeStore {
	return &fakeStore{now: now, byName: map[string]PINLogin{}, roles: map[account.ID]account.SiteRole{}, sessions: map[string]fakeSession{}}
}

func (f *fakeStore) CreatePINAccount(_ context.Context, _ account.DisplayName, digest []byte, pin PINHash) (account.ID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, taken := f.byName[string(digest)]; taken {
		return 0, ErrUsernameTaken
	}

	f.next++
	f.byName[string(digest)] = PINLogin{Account: f.next, UsernameDigest: digest, Hash: pin}
	f.roles[f.next] = account.SiteUser

	return f.next, nil
}

func (f *fakeStore) FindPINLogin(_ context.Context, digest []byte) (PINLogin, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lookups++
	login, ok := f.byName[string(digest)]

	return login, ok, nil
}

func (f *fakeStore) PINOfAccount(_ context.Context, id account.ID) (PINLogin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, login := range f.byName {
		if login.Account == id {
			return login, nil
		}
	}

	return PINLogin{}, errors.New("no such account")
}

func (f *fakeStore) ReplacePIN(_ context.Context, id account.ID, pin PINHash, mustChange bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for digest, login := range f.byName {
		if login.Account == id {
			f.byName[digest] = PINLogin{Account: id, UsernameDigest: login.UsernameDigest, Hash: pin, MustChange: mustChange}
			f.deleteSessionsOf(id)

			return nil
		}
	}

	return errors.New("no such account")
}

func (f *fakeStore) deleteSessionsOf(id account.ID) {
	for digest, s := range f.sessions {
		if s.account == id {
			delete(f.sessions, digest)
		}
	}
}

func (f *fakeStore) SiteRole(_ context.Context, id account.ID) (account.SiteRole, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.roles[id], nil
}

func (f *fakeStore) CreateSession(_ context.Context, digest []byte, id account.ID, checkedPIN []byte) error {
	if f.beforeSession != nil {
		f.beforeSession()
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, login := range f.byName {
		if login.Account == id && bytes.Equal(login.Hash.Digest, checkedPIN) {
			f.sessions[string(digest)] = fakeSession{account: id, lastUsed: f.now()}

			return nil
		}
	}

	return ErrWrongLogin
}

func (f *fakeStore) SessionAccount(_ context.Context, digest []byte, idle time.Duration) (Identity, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	s, ok := f.sessions[string(digest)]
	if !ok || f.now().Sub(s.lastUsed) >= idle {
		return Identity{}, false, nil
	}

	s.lastUsed = f.now()
	f.sessions[string(digest)] = s

	mustChange := false
	for _, login := range f.byName {
		if login.Account == s.account {
			mustChange = login.MustChange
		}
	}

	return Identity{Account: s.account, MustChangePIN: mustChange}, true, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, digest []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.sessions, string(digest))

	return nil
}

func (f *fakeStore) DeleteExpiredSessions(_ context.Context, idle time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var deleted int64

	for digest, s := range f.sessions {
		if f.now().Sub(s.lastUsed) >= idle {
			delete(f.sessions, digest)
			deleted++
		}
	}

	return deleted, nil
}

type clock struct {
	mu sync.Mutex
	at time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.at
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.at = c.at.Add(d)
}

func newTestService(t *testing.T) (*Service, *fakeStore, *clock) {
	t.Helper()

	c := &clock{at: t0}
	store := newFakeStore(c.now)

	return NewService(testKeys(t), store, c.now), store, c
}

var (
	ctx  = context.Background()
	home = ip("10.0.1.1")
)

func resolve(t *testing.T, s *Service, token Token) Identity {
	t.Helper()

	who, err := s.Resolve(ctx, token)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	return who
}
