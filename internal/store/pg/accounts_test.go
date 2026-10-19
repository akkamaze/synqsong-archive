package pg

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"synqsong/internal/account"
	"synqsong/internal/auth"
)

var accountKeys = func() auth.Keys {
	keys, err := auth.DeriveKeys(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		panic(err)
	}

	return keys
}()

var usernames = func() func() auth.Username {
	n := 0

	return func() auth.Username {
		n++
		u, err := auth.ParseUsername(fmt.Sprintf("user_%d_%d", time.Now().UnixNano()%1_000_000, n))
		if err != nil {
			panic(err)
		}

		return u
	}
}()

func newPINAccount(t *testing.T, s *Store, u auth.Username, pin string) account.ID {
	t.Helper()

	parsed, _ := auth.ParsePIN(pin)

	hash, err := accountKeys.HashPIN(parsed, rand.Reader)
	if err != nil {
		t.Fatalf("hash PIN: %v", err)
	}

	id, err := s.CreatePINAccount(context.Background(), "Ann", accountKeys.UsernameDigest(u), hash)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	return id
}

func TestAPINAccountCanBeFoundAndChecked(t *testing.T) {
	s := needDB(t)
	u := usernames()
	id := newPINAccount(t, s, u, "4821")

	login, found, err := s.FindPINLogin(context.Background(), accountKeys.UsernameDigest(u))
	if err != nil || !found || login.Account != id {
		t.Fatalf("find = %+v, %v, %v; want account %d", login, found, err, id)
	}

	right, _ := auth.ParsePIN("4821")
	wrong, _ := auth.ParsePIN("4822")

	if !accountKeys.VerifyPIN(right, login.Hash) || accountKeys.VerifyPIN(wrong, login.Hash) {
		t.Fatal("the stored PIN hash does not check PINs correctly")
	}

	profile, err := s.Profile(context.Background(), id)
	if err != nil || profile.Name != "Ann" || profile.Role != account.SiteUser {
		t.Fatalf("profile = %+v, %v; want Ann, a site user", profile, err)
	}
}

func TestAUsernameCanBeTakenOnlyOnce(t *testing.T) {
	s := needDB(t)
	u := usernames()
	newPINAccount(t, s, u, "1111")

	hash, _ := accountKeys.HashPIN("2222", rand.Reader)

	_, err := s.CreatePINAccount(context.Background(), "Copy", accountKeys.UsernameDigest(u), hash)
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("second account with the same username: %v, want ErrUsernameTaken", err)
	}
}

func TestTheUsernameItselfIsNeverStored(t *testing.T) {
	s := needDB(t)
	u := usernames()
	newPINAccount(t, s, u, "4821")

	var leaks int

	err := testPool.QueryRow(context.Background(), `
		SELECT count(*) FROM pin_logins
		WHERE position($1::bytea IN username_digest) > 0 OR position($1::bytea IN pin_digest) > 0`,
		[]byte(u)).Scan(&leaks)
	if err != nil || leaks != 0 {
		t.Fatalf("rows holding the plain username = %d (%v), want 0", leaks, err)
	}
}

func TestAnAdminSetPINMustBeChanged(t *testing.T) {
	s := needDB(t)
	u := usernames()
	id := newPINAccount(t, s, u, "4821")

	temp, _ := accountKeys.HashPIN("9302", rand.Reader)
	if err := s.SetPIN(context.Background(), id, temp, true); err != nil {
		t.Fatalf("set PIN: %v", err)
	}

	login, _, err := s.FindPINLogin(context.Background(), accountKeys.UsernameDigest(u))
	if err != nil || !login.MustChange || !accountKeys.VerifyPIN("9302", login.Hash) {
		t.Fatalf("after an admin set the PIN: %+v, %v; want the new PIN and must-change", login, err)
	}
}

func TestSessionsLastWhileUsedAndEndOnRequest(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	id := newPINAccount(t, s, usernames(), "4821")

	token, _ := auth.NewToken(rand.Reader)
	digest := auth.HashToken(token)

	if err := s.CreateSession(ctx, digest, id); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, found, err := s.SessionAccount(ctx, digest, time.Now().Add(-time.Hour))
	if err != nil || !found || got != id {
		t.Fatalf("session lookup = %d, %v, %v; want account %d", got, found, err, id)
	}

	if _, found, _ := s.SessionAccount(ctx, digest, time.Now().Add(time.Hour)); found {
		t.Fatal("a session not used since the cutoff was accepted, want it expired")
	}

	if err := s.DeleteSessionsOf(ctx, id); err != nil {
		t.Fatalf("delete sessions: %v", err)
	}

	if _, found, _ := s.SessionAccount(ctx, digest, time.Now().Add(-time.Hour)); found {
		t.Fatal("a session survived signing out everywhere")
	}
}

func TestOnlyTheTokenHashIsStored(t *testing.T) {
	s := needDB(t)
	ctx := context.Background()
	id := newPINAccount(t, s, usernames(), "4821")

	token, _ := auth.NewToken(rand.Reader)
	if err := s.CreateSession(ctx, auth.HashToken(token), id); err != nil {
		t.Fatalf("create session: %v", err)
	}

	var leaks int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE position($1::bytea IN token_digest) > 0`, []byte(token)).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatalf("rows holding the plain token = %d (%v), want 0", leaks, err)
	}
}
