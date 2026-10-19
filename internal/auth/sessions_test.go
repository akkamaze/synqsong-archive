package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

func TestASessionEndsAfterNinetyDaysUnused(t *testing.T) {
	s, _, c := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	c.advance(89 * 24 * time.Hour)
	if _, err := s.Resolve(ctx, up.Token); err != nil {
		t.Fatalf("session used after 89 days: %v, want alive", err)
	}

	c.advance(89 * 24 * time.Hour)
	if _, err := s.Resolve(ctx, up.Token); err != nil {
		t.Fatalf("session used again 89 days later: %v, want alive because it was used", err)
	}

	c.advance(91 * 24 * time.Hour)
	if _, err := s.Resolve(ctx, up.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("session unused for 91 days: %v, want ErrNoSession", err)
	}
}

func TestExpiredSessionsAreDeletedAfterNinetyDaysUnused(t *testing.T) {
	s, store, c := newTestService(t)
	old, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	c.advance(60 * 24 * time.Hour)
	fresh, _ := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "4821")

	c.advance(31 * 24 * time.Hour)

	if deleted, err := s.DeleteExpiredSessions(ctx); err != nil || deleted != 1 {
		t.Fatalf("delete expired = %d, %v; want the one unused for 91 days", deleted, err)
	}

	if _, kept := store.sessions[string(s.keys.TokenDigest(fresh.Token))]; !kept {
		t.Fatal("a session unused for 31 days was deleted")
	}

	if _, kept := store.sessions[string(s.keys.TokenDigest(old.Token))]; kept {
		t.Fatal("a session unused for 91 days was kept")
	}
}

func TestSignOutEndsTheSession(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	if err := s.SignOut(ctx, up.Token); err != nil {
		t.Fatalf("sign out: %v", err)
	}

	if _, err := s.Resolve(ctx, up.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("resolve after sign out: %v, want ErrNoSession", err)
	}
}

func TestASessionWrittenWithoutTheKeyIsRefused(t *testing.T) {
	s, store, c := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	forged, _ := NewToken(rand.Reader, up.Identity.Account)
	plain := sha256.Sum256([]byte(forged))
	store.sessions[string(plain[:])] = fakeSession{account: up.Identity.Account, lastUsed: c.now()}

	if _, err := s.Resolve(ctx, forged); !errors.Is(err, ErrNoSession) {
		t.Fatalf("a session row made from the token's plain SHA-256: %v, want ErrNoSession", err)
	}
}

func TestMovingASessionRowToAnotherAccountVoidsIt(t *testing.T) {
	s, store, _ := newTestService(t)
	ann, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	mallory, _ := s.SignUp(ctx, ip(attacker), "mallory", "1111", "Mallory")

	// What someone running SQL as the app could do: point their own session at Ann.
	row := string(s.keys.TokenDigest(mallory.Token))
	moved := store.sessions[row]
	moved.account = ann.Identity.Account
	store.sessions[row] = moved

	if who, err := s.Resolve(ctx, mallory.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("a session row moved to Ann resolves to %+v, %v; want ErrNoSession", who, err)
	}
}
