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

func pinHash(t *testing.T, u auth.Username, pin string) auth.PINHash {
	t.Helper()

	parsed, _ := auth.ParsePIN(pin)

	hash, err := accountKeys.HashPIN(parsed, accountKeys.UsernameDigest(u), rand.Reader)
	if err != nil {
		t.Fatalf("hash PIN: %v", err)
	}

	return hash
}

func pinIs(u auth.Username, pin auth.PIN, hash auth.PINHash) bool {
	return accountKeys.VerifyPIN(pin, accountKeys.UsernameDigest(u), hash)
}

func newPINAccount(t *testing.T, s *Store, u auth.Username, pin string) account.ID {
	t.Helper()

	id, err := s.CreatePINAccount(context.Background(), "Ann", accountKeys.UsernameDigest(u), pinHash(t, u, pin))
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

	if !pinIs(u, right, login.Hash) || pinIs(u, wrong, login.Hash) {
		t.Fatal("the stored PIN hash does not check PINs correctly")
	}

	role, err := s.SiteRole(context.Background(), id)
	if err != nil || role != account.SiteUser {
		t.Fatalf("site role = %d, %v; want a site user", role, err)
	}
}

func TestAUsernameCanBeTakenOnlyOnce(t *testing.T) {
	s := needDB(t)
	u := usernames()
	newPINAccount(t, s, u, "1111")

	_, err := s.CreatePINAccount(context.Background(), "Copy", accountKeys.UsernameDigest(u), pinHash(t, u, "2222"))
	if !errors.Is(err, auth.ErrUsernameTaken) {
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

func TestThePINOfAnAccountCarriesItsUsernameDigest(t *testing.T) {
	s := needDB(t)
	u := usernames()
	id := newPINAccount(t, s, u, "4821")

	login, err := s.PINOfAccount(context.Background(), id)
	if err != nil || login.Account != id || !bytes.Equal(login.UsernameDigest, accountKeys.UsernameDigest(u)) || !pinIs(u, "4821", login.Hash) {
		t.Fatalf("PIN of account = %+v, %v; want its username digest and PIN", login, err)
	}

	found, _, err := s.FindPINLogin(context.Background(), accountKeys.UsernameDigest(u))
	if err != nil || !bytes.Equal(found.UsernameDigest, login.UsernameDigest) {
		t.Fatalf("find = %+v, %v; want the same username digest", found, err)
	}
}
