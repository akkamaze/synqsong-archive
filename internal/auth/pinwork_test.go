package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"synqsong/internal/account"
)

func scryptRuns(s *Service, do func()) int64 {
	before := s.pins.runs.Load()
	do()

	return s.pins.runs.Load() - before
}

func TestAnUnknownNameRunsScryptAsOftenAsAWrongPIN(t *testing.T) {
	s, _, _ := newTestService(t)
	_, _ = s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	wrongPIN := scryptRuns(s, func() { _, _ = s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "0000") })
	unknown := scryptRuns(s, func() { _, _ = s.SignIn(ctx, ip("10.0.0.1"), nobody, "nobody_here", "0000") })

	if wrongPIN != 1 || unknown != wrongPIN {
		t.Fatalf("scrypt runs: wrong PIN %d, unknown name %d; want 1 each so both take as long", wrongPIN, unknown)
	}
}

func TestSignInWaitsForAFreeScryptSlotOnlyAsLongAsTheRequest(t *testing.T) {
	s, _, _ := newTestService(t)
	_, _ = s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	for range cap(s.pins.slots) {
		s.pins.slots <- struct{}{}
	}

	waiting, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()

	if _, err := s.SignIn(waiting, ip("10.0.0.1"), nobody, "ann_1", "4821"); !errors.Is(err, ErrBusy) {
		t.Fatalf("sign-in while every scrypt slot is taken: %v, want ErrBusy", err)
	}

	for range cap(s.pins.slots) {
		<-s.pins.slots
	}

	for range 5 {
		_, _ = s.SignIn(waiting, ip("10.0.0.1"), nobody, "ann_1", "4821")
	}

	if _, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "4821"); err != nil {
		t.Fatalf("sign-in after 6 busy refusals: %v, want them not counted as wrong PINs", err)
	}
}

func TestEveryPINHashWaitsForAScryptSlot(t *testing.T) {
	s, store, _ := newTestService(t)
	boss, _ := s.SignUp(ctx, home, "boss_1", "1357", "Boss")
	store.roles[boss.Identity.Account] = account.SiteAdmin
	admin := resolve(t, s, boss.Token)

	for range cap(s.pins.slots) {
		s.pins.slots <- struct{}{}
	}
	defer func() {
		for range cap(s.pins.slots) {
			<-s.pins.slots
		}
	}()

	waiting, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()

	_, signUp := s.SignUp(waiting, home, "ann_1", "4821", "Ann")
	_, change := s.ChangePIN(waiting, home, nobody, admin, "1357", "2468")
	_, reset := s.ResetPIN(waiting, admin, admin.Account)

	for name, err := range map[string]error{"sign-up": signUp, "PIN change": change, "PIN reset": reset} {
		if !errors.Is(err, ErrBusy) {
			t.Errorf("%s while every scrypt slot is taken: %v, want ErrBusy", name, err)
		}
	}
}
