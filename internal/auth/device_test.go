package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"synqsong/internal/account"
)

const (
	attacker = "198.51.100.66"
	nobody   = DeviceToken("")
)

func lockByGuessing(t *testing.T, s *Service, raw string) {
	t.Helper()

	for range 5 {
		_, _ = s.SignIn(ctx, ip(attacker), nobody, raw, "0000")
	}

	if _, err := s.SignIn(ctx, ip(attacker), nobody, raw, "0000"); lockedFor(err) == 0 {
		t.Fatalf("username after 5 wrong PINs from elsewhere: %v, want locked", err)
	}
}

func TestADeviceThatSignedInBeforeIsNotLockedOutByGuessesElsewhere(t *testing.T) {
	s, _, c := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	if up.Device == "" {
		t.Fatal("sign-up gave no device token")
	}

	device := up.Device

	for round := range 3 {
		lockByGuessing(t, s, "ann_1")

		in, err := s.SignIn(ctx, home, device, "ann_1", "4821")
		if err != nil {
			t.Fatalf("round %d: owner's device while the username is locked: %v, want signed in", round+1, err)
		}

		device = in.Device
		c.advance(time.Hour)
	}
}

func TestAForgedOrOtherAccountsDeviceTokenFallsBackToTheUsernameLock(t *testing.T) {
	s, store, c := newTestService(t)
	ann, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	mallory, _ := s.SignUp(ctx, ip(attacker), "mallory", "1111", "Mallory")
	annDigest := digestOf(t, s.keys, "ann_1")
	annPIN := store.byName[string(annDigest)].Hash
	lockByGuessing(t, s, "ann_1")

	raw, _ := base64.RawURLEncoding.DecodeString(string(ann.Device))
	flipped := bytes.Clone(raw)
	flipped[len(flipped)-1] ^= 1

	otherKeys, _ := DeriveKeys(bytes.Repeat([]byte{8}, 32))
	elsewhere, _ := otherKeys.newDeviceToken(annDigest, annPIN, c.now(), rand.Reader)
	expired, _ := s.keys.newDeviceToken(annDigest, annPIN, c.now().Add(-deviceLife), rand.Reader)

	tokens := map[string]DeviceToken{
		"another account's token":       mallory.Device,
		"a token with one bit flipped":  DeviceToken(base64.RawURLEncoding.EncodeToString(flipped)),
		"a token cut short":             ann.Device[:20],
		"not base64":                    "!!not a token!!",
		"a token made with another key": elsewhere,
		"a token older than a year":     expired,
	}

	for what, token := range tokens {
		if _, err := s.SignIn(ctx, home, token, "ann_1", "4821"); lockedFor(err) == 0 {
			t.Errorf("%s: %v, want the username lock to apply", what, err)
		}
	}
}

func TestADevicesOwnWrongTriesLockOnlyThatDevice(t *testing.T) {
	s, _, _ := newTestService(t)
	first, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	second, _ := s.SignIn(ctx, ip("10.0.2.2"), nobody, "ann_1", "4821")

	for range 5 {
		_, _ = s.SignIn(ctx, home, first.Device, "ann_1", "0000")
	}

	if _, err := s.SignIn(ctx, home, first.Device, "ann_1", "4821"); lockedFor(err) != 10*time.Second {
		t.Fatalf("the device after its own 5 wrong PINs: %v, want locked for 10s", err)
	}

	if _, err := s.SignIn(ctx, ip("10.0.2.2"), second.Device, "ann_1", "4821"); err != nil {
		t.Fatalf("another device of the same account: %v, want signed in", err)
	}

	if _, err := s.SignIn(ctx, ip("10.0.3.3"), nobody, "ann_1", "4821"); err != nil {
		t.Fatalf("a sign-in without a device token: %v, want the username unlocked", err)
	}
}

func TestADeviceTokenStillCountsTowardItsAddress(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")

	for i := range 20 {
		_, _ = s.SignIn(ctx, ip(attacker), nobody, "name_"+string(rune('a'+i)), "0000")
	}

	if _, err := s.SignIn(ctx, ip(attacker), up.Device, "ann_1", "4821"); lockedFor(err) == 0 {
		t.Fatalf("a device token from a locked address: %v, want the address lock to apply", err)
	}
}

func TestADeviceCanChangeThePINWhileTheUsernameIsLocked(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	lockByGuessing(t, s, "ann_1")

	if _, err := s.ChangePIN(ctx, home, up.Device, up.Identity, "4821", "2468"); err != nil {
		t.Fatalf("PIN change from the owner's device while the username is locked: %v, want allowed", err)
	}
}

func TestAnAdminResetLiftsTheLocksOnTheAccountsDevices(t *testing.T) {
	s, store, _ := newTestService(t)
	boss, _ := s.SignUp(ctx, home, "boss_1", "1357", "Boss")
	store.roles[boss.Identity.Account] = account.SiteAdmin
	ann, _ := s.SignUp(ctx, ip("10.0.2.2"), "ann_1", "4821", "Ann")

	for range 5 {
		_, _ = s.SignIn(ctx, ip("10.0.2.2"), ann.Device, "ann_1", "0000")
	}

	temp, err := s.ResetPIN(ctx, resolve(t, s, boss.Token), ann.Identity.Account)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}

	if _, err := s.SignIn(ctx, ip("10.0.2.2"), ann.Device, "ann_1", string(temp)); err != nil {
		t.Fatalf("one-time PIN from the device that was locked: %v, want allowed", err)
	}
}

func TestADeviceTokenHoldsNoUsernameOrAccount(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	raw, _ := base64.RawURLEncoding.DecodeString(string(up.Device))

	if bytes.Contains(raw, []byte("ann_1")) || bytes.Contains(raw, digestOf(t, s.keys, "ann_1")) {
		t.Fatal("the device token carries the username or its digest")
	}

	if _, err := s.Resolve(ctx, Token(up.Device)); !errors.Is(err, ErrNoSession) {
		t.Fatalf("a device token used as a session: %v, want ErrNoSession", err)
	}
}

func TestDeviceTokensFromBeforeAPINChangeCountAsNone(t *testing.T) {
	s, _, _ := newTestService(t)
	up, _ := s.SignUp(ctx, home, "ann_1", "4821", "Ann")
	old := up.Device

	changed, err := s.ChangePIN(ctx, home, up.Device, up.Identity, "4821", "2468")
	if err != nil {
		t.Fatalf("change PIN: %v", err)
	}

	lockByGuessing(t, s, "ann_1")

	// Someone who once knew the PIN must not keep their own guessing counters after it changes.
	if _, err := s.SignIn(ctx, ip(attacker), old, "ann_1", "0000"); lockedFor(err) == 0 {
		t.Fatalf("a token from before the PIN change: %v, want the username lock to apply", err)
	}

	if _, err := s.SignIn(ctx, home, changed.Device, "ann_1", "2468"); err != nil {
		t.Fatalf("the token from the change itself: %v, want signed in", err)
	}
}
