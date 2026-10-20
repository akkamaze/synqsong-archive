package auth

import (
	"errors"
	"testing"
	"time"

	"synqsong/internal/account"
)

func TestAnAdminResetPINMustBeChangedAndSignsOutEverywhere(t *testing.T) {
	s, store, _ := newTestService(t)
	boss, _ := s.SignUp(ctx, home, "boss_1", "1357", "Boss")
	store.roles[boss.Identity.Account] = account.SiteAdmin
	ann, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	admin := resolve(t, s, boss.Token)

	if _, err := s.ResetPIN(ctx, resolve(t, s, ann.Token), admin.Account); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("reset by a non-admin: %v, want ErrNotAdmin", err)
	}

	temp, err := s.ResetPIN(ctx, admin, ann.Identity.Account)
	if err != nil || len(temp) != 4 {
		t.Fatalf("reset = %q, %v; want a 4-digit one-time PIN", temp, err)
	}

	if _, err := s.Resolve(ctx, ann.Token); !errors.Is(err, ErrNoSession) {
		t.Fatal("the user's old session survived the reset")
	}

	in, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", string(temp))
	if err != nil || !in.Identity.MustChangePIN {
		t.Fatalf("sign in with the one-time PIN = %+v, %v; want must change", in, err)
	}

	if _, err := s.Resolve(ctx, in.Token); !errors.Is(err, ErrMustChangePIN) {
		t.Fatalf("using the one-time PIN session for anything else: %v, want ErrMustChangePIN", err)
	}

	forced, err := s.ResolveForPINChange(ctx, in.Token)
	if err != nil || !forced.MustChangePIN {
		t.Fatalf("resolve for a PIN change = %+v, %v; want the forced change", forced, err)
	}

	if _, err := s.ChangePIN(ctx, ip("10.0.0.1"), nobody, forced, string(temp), string(temp)); !errors.Is(err, ErrPINMustDiffer) {
		t.Fatalf("keeping the one-time PIN: %v, want ErrPINMustDiffer", err)
	}

	changed, err := s.ChangePIN(ctx, ip("10.0.0.1"), nobody, forced, string(temp), "2468")
	if err != nil || changed.Identity.MustChangePIN {
		t.Fatalf("change = %+v, %v; want a normal session", changed, err)
	}

	if _, err := s.Resolve(ctx, changed.Token); err != nil {
		t.Fatalf("the session after the change: %v, want usable", err)
	}

	if _, err := s.ResolveForPINChange(ctx, in.Token); !errors.Is(err, ErrNoSession) {
		t.Fatal("the one-time PIN session survived the change")
	}

	if _, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "2468"); err != nil {
		t.Fatalf("sign in with the chosen PIN: %v", err)
	}
}

func TestAnAdminUnderAForcedPINChangeCannotResetOthers(t *testing.T) {
	s, store, _ := newTestService(t)
	boss, _ := s.SignUp(ctx, home, "boss_1", "1357", "Boss")
	store.roles[boss.Identity.Account] = account.SiteAdmin
	ann, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	admin := resolve(t, s, boss.Token)

	temp, _ := s.ResetPIN(ctx, admin, boss.Identity.Account)
	in, _ := s.SignIn(ctx, ip("10.0.0.1"), nobody, "boss_1", string(temp))

	forced, err := s.ResolveForPINChange(ctx, in.Token)
	if err != nil {
		t.Fatalf("resolve for a PIN change: %v", err)
	}

	if _, err := s.ResetPIN(ctx, forced, ann.Identity.Account); !errors.Is(err, ErrMustChangePIN) {
		t.Fatalf("reset by an admin who must change their PIN: %v, want ErrMustChangePIN", err)
	}

	if _, err := s.Resolve(ctx, ann.Token); err != nil {
		t.Fatalf("the user's session after a refused reset: %v, want it kept", err)
	}
}

func TestWrongPINsInAPINChangeLockSignInToo(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	for range 5 {
		_, _ = s.ChangePIN(ctx, ip("10.0.0.1"), nobody, up.Identity, "0000", "2468")
	}

	var locked *LockedError
	if _, err := s.SignIn(ctx, ip("10.0.0.2"), nobody, "ann_1", "4821"); !errors.As(err, &locked) {
		t.Fatalf("sign-in after 5 wrong PINs in a PIN change: %v, want LockedError", err)
	}
}

func TestWrongSignInsLockAPINChangeToo(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	for range 5 {
		_, _ = s.SignIn(ctx, ip("10.0.0.2"), nobody, "ann_1", "0000")
	}

	var locked *LockedError
	if _, err := s.ChangePIN(ctx, ip("10.0.0.1"), nobody, up.Identity, "4821", "2468"); !errors.As(err, &locked) {
		t.Fatalf("PIN change after 5 wrong sign-ins: %v, want LockedError", err)
	}
}

func TestChangingThePINNeedsTheCurrentOne(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	if _, err := s.ChangePIN(ctx, ip("10.0.0.1"), nobody, up.Identity, "0000", "2468"); !errors.Is(err, ErrWrongLogin) {
		t.Fatalf("change with a wrong current PIN: %v, want ErrWrongLogin", err)
	}

	if _, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "4821"); err != nil {
		t.Fatalf("old PIN after a refused change: %v, want it still working", err)
	}
}

func TestAnAdminResetLiftsTheLockOnTheUsername(t *testing.T) {
	s, store, c := newTestService(t)
	boss, _ := s.SignUp(ctx, home, "boss_1", "1357", "Boss")
	store.roles[boss.Identity.Account] = account.SiteAdmin
	ann, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	admin := resolve(t, s, boss.Token)
	name := usernameKey(digestOf(t, s.keys, "ann_1"))

	for i, lock := range growingLocks {
		for range 5 {
			_ = fail(s.limits, c.now(), name)
		}

		if i < len(growingLocks)-1 {
			c.advance(lock)
		}
	}

	if _, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", "4821"); lockedFor(err) != time.Hour {
		t.Fatalf("sign-in after five locks: %v, want locked for 1h", err)
	}

	temp, err := s.ResetPIN(ctx, admin, ann.Identity.Account)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}

	if _, err := s.SignIn(ctx, ip("10.0.0.1"), nobody, "ann_1", string(temp)); err != nil {
		t.Fatalf("sign in with the one-time PIN while the old lock would still run: %v, want allowed", err)
	}
}

func TestAMalformedCurrentPINIsRefusedWithoutCountingATry(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	// No PIN is checked, so these must not cost the attacker nothing while locking the name.
	for range 10 {
		if _, err := s.ChangePIN(ctx, ip(attacker), nobody, up.Identity, "12", "2468"); !errors.Is(err, ErrWrongLogin) {
			t.Fatalf("change with a malformed current PIN: %v, want ErrWrongLogin", err)
		}
	}

	if _, err := s.SignIn(ctx, ip("10.0.3.3"), nobody, "ann_1", "4821"); err != nil {
		t.Fatalf("sign in after malformed PIN changes: %v, want the name not locked", err)
	}
}
