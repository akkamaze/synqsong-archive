package pg

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"synqsong/internal/account"
	"synqsong/internal/auth"
)

const ninetyDays = 90 * 24 * time.Hour

func newSession(t *testing.T, s *Store, id account.ID) []byte {
	t.Helper()

	token, _ := auth.NewToken(rand.Reader, id)
	digest := accountKeys.TokenDigest(token)

	if err := s.CreateSession(context.Background(), digest, id, pinDigestOf(t, id)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	return digest
}

func pinDigestOf(t *testing.T, id account.ID) []byte {
	t.Helper()

	var digest []byte
	if err := testPool.QueryRow(context.Background(), `SELECT pin_digest FROM pin_logins WHERE account_id = $1`, int64(id)).Scan(&digest); err != nil {
		t.Fatalf("read PIN digest: %v", err)
	}

	return digest
}

func sessionExists(t *testing.T, digest []byte) bool {
	t.Helper()

	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM sessions WHERE token_digest = $1`, digest).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}

	return n > 0
}

func setLastUsed(t *testing.T, digest []byte, ago time.Duration) {
	t.Helper()

	if _, err := testPool.Exec(context.Background(), `UPDATE sessions SET last_used_at = now() - $2::interval WHERE token_digest = $1`, digest, ago); err != nil {
		t.Fatalf("age session: %v", err)
	}
}

func lastUsedAgo(t *testing.T, digest []byte) time.Duration {
	t.Helper()

	var ago time.Duration
	if err := testPool.QueryRow(context.Background(), `SELECT now() - last_used_at FROM sessions WHERE token_digest = $1`, digest).Scan(&ago); err != nil {
		t.Fatalf("read session age: %v", err)
	}

	return ago
}

func TestASessionLastsWhileUsedWithinItsIdleTime(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	id := newPINAccount(t, s, usernames(), "4821")
	digest := newSession(t, s, id)

	setLastUsed(t, digest, 2*time.Hour)

	got, found, err := s.SessionAccount(ctx, digest, 3*time.Hour)
	if err != nil || !found || got.Account != id || got.MustChangePIN {
		t.Fatalf("session used 2h ago, idle 3h = %+v, %v, %v; want account %d without a forced PIN change", got, found, err, id)
	}

	setLastUsed(t, digest, 4*time.Hour)

	if _, found, _ := s.SessionAccount(ctx, digest, 3*time.Hour); found {
		t.Fatal("a session unused for 4h was accepted with a 3h idle time, want it expired")
	}
}

func TestASessionIsMarkedUsedAtMostOnceAnHour(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	digest := newSession(t, s, newPINAccount(t, s, usernames(), "4821"))

	setLastUsed(t, digest, 30*time.Minute)

	if _, found, err := s.SessionAccount(ctx, digest, ninetyDays); err != nil || !found {
		t.Fatalf("session lookup: %v, %v", found, err)
	}

	if ago := lastUsedAgo(t, digest); ago < 29*time.Minute {
		t.Fatalf("last used %v ago after a lookup 30 min after the last write, want it left alone", ago)
	}

	setLastUsed(t, digest, 61*time.Minute)

	if _, found, err := s.SessionAccount(ctx, digest, ninetyDays); err != nil || !found {
		t.Fatalf("session lookup: %v, %v", found, err)
	}

	if ago := lastUsedAgo(t, digest); ago > time.Minute {
		t.Fatalf("last used %v ago after a lookup 61 min after the last write, want it marked now", ago)
	}
}

func TestDeleteExpiredSessionsKeepsLiveOnes(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	id := newPINAccount(t, s, usernames(), "4821")
	stale, live := newSession(t, s, id), newSession(t, s, id)

	setLastUsed(t, stale, ninetyDays+time.Hour)
	setLastUsed(t, live, ninetyDays-time.Hour)

	deleted, err := s.DeleteExpiredSessions(ctx, ninetyDays)
	if err != nil || deleted < 1 {
		t.Fatalf("delete expired = %d, %v; want at least the stale one", deleted, err)
	}

	if sessionExists(t, stale) {
		t.Fatal("a session unused for over 90 days was kept")
	}

	if _, found, _ := s.SessionAccount(ctx, live, ninetyDays); !found {
		t.Fatal("a session used 89 days ago was deleted, want it kept")
	}
}

func TestReplacingAPINSignsOutEverywhere(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	u := usernames()
	id := newPINAccount(t, s, u, "4821")
	digest := newSession(t, s, id)

	if err := s.ReplacePIN(ctx, id, pinHash(t, u, "9302"), true); err != nil {
		t.Fatalf("replace PIN: %v", err)
	}

	login, _, err := s.FindPINLogin(ctx, accountKeys.UsernameDigest(u))
	if err != nil || !login.MustChange || !pinIs(u, "9302", login.Hash) {
		t.Fatalf("after an admin set the PIN: %+v, %v; want the new PIN and must-change", login, err)
	}

	if sessionExists(t, digest) {
		t.Fatal("a session survived the PIN being replaced")
	}
}

func TestASessionNeedsThePINThatWasChecked(t *testing.T) {
	s := needDB(t)
	u := usernames()
	id := newPINAccount(t, s, u, "4821")
	stale := pinDigestOf(t, id)

	if err := s.ReplacePIN(context.Background(), id, pinHash(t, u, "9302"), false); err != nil {
		t.Fatalf("replace PIN: %v", err)
	}

	token, _ := auth.NewToken(rand.Reader, id)
	digest := accountKeys.TokenDigest(token)

	if err := s.CreateSession(context.Background(), digest, id, stale); !errors.Is(err, auth.ErrWrongLogin) {
		t.Fatalf("session for a PIN that was replaced: %v, want ErrWrongLogin", err)
	}

	if sessionExists(t, digest) {
		t.Fatal("a session was stored for a PIN that is no longer the account's")
	}
}

// The interleaving that leaks a session: the PIN change has updated the row and deleted
// sessions but not committed when the sign-in inserts its session.
func TestASignInRacingAPINChangeLeavesNoSession(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	u := usernames()
	id := newPINAccount(t, s, u, "4821")
	checked := pinDigestOf(t, id)

	change, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin PIN change: %v", err)
	}
	defer func() { _ = change.Rollback(ctx) }()

	replaced := pinHash(t, u, "9302")
	if _, err := change.Exec(ctx, `UPDATE pin_logins SET pin_salt = $2, pin_digest = $3 WHERE account_id = $1`, int64(id), replaced.Salt, replaced.Digest); err != nil {
		t.Fatalf("change PIN: %v", err)
	}

	if _, err := change.Exec(ctx, `DELETE FROM sessions WHERE account_id = $1`, int64(id)); err != nil {
		t.Fatalf("sign out everywhere: %v", err)
	}

	token, _ := auth.NewToken(rand.Reader, id)
	digest := accountKeys.TokenDigest(token)
	signedIn := make(chan error, 1)

	go func() { signedIn <- s.CreateSession(ctx, digest, id, checked) }()

	select {
	case err := <-signedIn:
		signedIn <- err
	case <-time.After(500 * time.Millisecond):
	}

	if err := change.Commit(ctx); err != nil {
		t.Fatalf("commit PIN change: %v", err)
	}

	if err := <-signedIn; !errors.Is(err, auth.ErrWrongLogin) {
		t.Fatalf("sign-in racing the PIN change: %v, want ErrWrongLogin", err)
	}

	if sessionExists(t, digest) {
		t.Fatal("a session made with the old PIN outlived the PIN change")
	}
}

func TestOnlyTheTokenDigestIsStored(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	id := newPINAccount(t, s, usernames(), "4821")

	token, _ := auth.NewToken(rand.Reader, id)
	if err := s.CreateSession(ctx, accountKeys.TokenDigest(token), id, pinDigestOf(t, id)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	var leaks int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE position($1::bytea IN token_digest) > 0`, []byte(token)).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatalf("rows holding the plain token = %d (%v), want 0", leaks, err)
	}
}

func TestASessionCarriesTheForcedPINChange(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	u := usernames()
	id := newPINAccount(t, s, u, "4821")

	if err := s.ReplacePIN(ctx, id, pinHash(t, u, "9302"), true); err != nil {
		t.Fatalf("replace PIN: %v", err)
	}

	stored, err := s.PINOfAccount(ctx, id)
	if err != nil || !pinIs(u, "9302", stored.Hash) {
		t.Fatalf("PIN of account = %v; want the one just set", err)
	}

	who, found, err := s.SessionAccount(ctx, newSession(t, s, id), ninetyDays)
	if err != nil || !found || !who.MustChangePIN {
		t.Fatalf("session = %+v, %v, %v; want it to say the PIN must change", who, found, err)
	}
}
