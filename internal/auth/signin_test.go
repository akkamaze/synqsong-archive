package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSignUpThenSignIn(t *testing.T) {
	s, _, _ := newTestService(t)

	up, err := s.SignUp(ctx, home, "Ann_1", "4821", "Ann")
	if err != nil {
		t.Fatalf("sign up: %v", err)
	}

	in, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "4821")
	if err != nil || in.Identity.Account != up.Identity.Account {
		t.Fatalf("sign in with another case = %+v, %v; want the same account", in, err)
	}

	if in.Token == up.Token {
		t.Fatal("two sign-ins share one token")
	}

	if _, err := s.SignUp(ctx, home, "ANN_1", "1111", "Copy"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("sign up with the same name in other case: %v, want ErrUsernameTaken", err)
	}
}

func TestAWrongPINAndAnUnknownNameLookTheSame(t *testing.T) {
	s, _, _ := newTestService(t)
	_, _ = s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	_, wrongPIN := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "0000")
	_, unknown := s.SignIn(ctx, ip("10.0.0.1"), nobody, "nobody_here", "4821")
	_, malformed := s.SignIn(ctx, ip("10.0.0.1"), nobody, "x", "12")

	for name, err := range map[string]error{"wrong PIN": wrongPIN, "unknown name": unknown, "malformed": malformed} {
		if !errors.Is(err, ErrWrongLogin) {
			t.Errorf("%s: %v, want ErrWrongLogin", name, err)
		}
	}
}

func TestSixWrongPINsLockTheNameEvenWithTheRightPIN(t *testing.T) {
	s, _, c := newTestService(t)
	_, _ = s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	for range 5 {
		_, _ = s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "0000")
	}

	var locked *LockedError
	if _, err := s.SignIn(ctx, ip("10.0.0.2"), nobody, "ann_1", "4821"); !errors.As(err, &locked) {
		t.Fatalf("right PIN while locked: %v, want LockedError", err)
	}

	c.advance(10 * time.Second)

	if _, err := s.SignIn(ctx, ip("10.0.0.2"), nobody, "ann_1", "4821"); err != nil {
		t.Fatalf("right PIN after the lock: %v", err)
	}
}

func TestManyAttemptsAtOnceGetNoMoreTriesThanTheLimit(t *testing.T) {
	s, _, _ := newTestService(t)
	_, _ = s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	checked := scryptRuns(s, func() {
		var wg sync.WaitGroup
		for range 40 {
			wg.Go(func() { _, _ = s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "0000") })
		}
		wg.Wait()
	})

	if checked > 5 {
		t.Fatalf("PINs checked = %d for 40 parallel tries, want at most 5", checked)
	}
}

func TestOneAddressTryingManyNamesIsLocked(t *testing.T) {
	s, _, _ := newTestService(t)

	for i := range 20 {
		_, _ = s.SignIn(ctx, ip("10.0.0.9"), nobody, "name_"+string(rune('a'+i)), "0000")
	}

	var locked *LockedError
	if _, err := s.SignIn(ctx, ip("10.0.0.9"), nobody, "name_z", "0000"); !errors.As(err, &locked) {
		t.Fatalf("21st try from one address: %v, want LockedError", err)
	}
}

func TestSignInsThatSucceedNeverLockTheirAddress(t *testing.T) {
	s, _, _ := newTestService(t)
	_, _ = s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	for i := range 25 {
		if _, err := s.SignIn(ctx, ip("10.0.0.9"), nobody, "ann_1", "4821"); err != nil {
			t.Fatalf("sign-in %d from one address: %v, want successes never counted", i+1, err)
		}
	}
}

func TestOneAddressMakesAtMostSixtyAccountsADay(t *testing.T) {
	s, _, _ := newTestService(t)

	for i := range 59 {
		if _, err := s.SignUp(ctx, home, fmt.Sprintf("ann_%d", i), "4821", "Ann"); err != nil {
			t.Fatalf("sign-up %d: %v", i+1, err)
		}
	}

	if _, err := s.SignUp(ctx, home, "ann_0", "4821", "Ann"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("taken name: %v, want ErrUsernameTaken", err)
	}

	if _, err := s.SignUp(ctx, home, "ann_60", "4821", "Ann"); lockedFor(err) != 24*time.Hour {
		t.Fatalf("61st sign-up from one address, counting the taken name: %v, want locked for 24h", err)
	}

	if _, err := s.SignUp(ctx, ip("10.0.1.2"), "bob_1", "4821", "Bob"); err != nil {
		t.Fatalf("sign-up from another address: %v, want allowed", err)
	}
}

func TestASignInRacingAPINChangeGetsNoSession(t *testing.T) {
	s, store, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	store.beforeSession = func() {
		replaced, _ := testKeys(t).HashPIN("9302", digestOf(t, testKeys(t), "ann_1"), rand.Reader)
		_ = store.ReplacePIN(ctx, up.Identity.Account, replaced, true)
	}

	if _, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "4821"); !errors.Is(err, ErrWrongLogin) {
		t.Fatalf("sign-in whose PIN was replaced after the check: %v, want ErrWrongLogin", err)
	}

	if len(store.sessions) != 0 {
		t.Fatalf("sessions after the race = %d, want none", len(store.sessions))
	}
}

func TestAPINCopiedOntoAnotherLoginDoesNotSignIn(t *testing.T) {
	s, store, _ := newTestService(t)
	_, _ = s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	_, _ = s.SignUp(ctx, home, "mallory", "1111", "Mallory")

	ann, mallory := digestOf(t, s.keys, "ann_1"), digestOf(t, s.keys, "mallory")

	victim := store.byName[string(ann)]
	victim.Hash = store.byName[string(mallory)].Hash
	store.byName[string(ann)] = victim

	if _, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "1111"); !errors.Is(err, ErrWrongLogin) {
		t.Fatalf("sign in as ann with mallory's PIN copied onto her login: %v, want ErrWrongLogin", err)
	}
}
