package auth

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"synqsong/internal/account"
)

// ChangePIN sets a new PIN once the current one is checked, signs out every other device
// and returns a fresh session. Wrong current PINs count toward the same lock as sign-in.
func (s *Service) ChangePIN(ctx context.Context, from ClientAddr, device DeviceToken, who Identity, rawCurrent, rawNew string) (Session, error) {
	current, err := ParsePIN(rawCurrent)
	if err != nil {
		return Session{}, ErrWrongLogin
	}

	next, err := ParsePIN(rawNew)
	if err != nil {
		return Session{}, err
	}

	stored, err := s.store.PINOfAccount(ctx, who.Account)
	if err != nil {
		return Session{}, err
	}

	attempt, err := s.limits.Reserve(s.now(), s.guessKeys(stored.UsernameDigest, stored.Hash, device, from)...)
	if err != nil {
		return Session{}, err
	}
	defer attempt.Release()

	right, err := s.pins.verify(ctx, current, stored.UsernameDigest, stored.Hash)
	if err != nil {
		return Session{}, err
	}

	if !right {
		attempt.Fail(s.now())

		return Session{}, ErrWrongLogin
	}

	attempt.Succeed(s.now())

	if stored.MustChange && next == current {
		return Session{}, ErrPINMustDiffer
	}

	hash, err := s.pins.hash(ctx, next, stored.UsernameDigest, s.random)
	if err != nil {
		return Session{}, err
	}

	if err := s.store.ReplacePIN(ctx, who.Account, hash, false); err != nil {
		return Session{}, err
	}

	return s.startSession(ctx, Identity{Account: who.Account}, stored.UsernameDigest, hash)
}

// ResetPIN lets a site admin, as returned by Resolve, give someone a one-time PIN. It signs
// them out everywhere and they must choose their own at next sign-in, so the admin never
// knows the PIN in use. It lifts the locks on their username and devices so the one-time PIN
// works at once.
func (s *Service) ResetPIN(ctx context.Context, admin Identity, target account.ID) (PIN, error) {
	if admin.MustChangePIN {
		return "", ErrMustChangePIN
	}

	role, err := s.store.SiteRole(ctx, admin.Account)
	if err != nil {
		return "", err
	}

	if role != account.SiteAdmin {
		return "", ErrNotAdmin
	}

	login, err := s.store.PINOfAccount(ctx, target)
	if err != nil {
		return "", err
	}

	temp, err := randomPIN(s.random)
	if err != nil {
		return "", err
	}

	hash, err := s.pins.hash(ctx, temp, login.UsernameDigest, s.random)
	if err != nil {
		return "", err
	}

	if err := s.store.ReplacePIN(ctx, target, hash, true); err != nil {
		return "", err
	}

	s.limits.ClearLogin(login.UsernameDigest)

	return temp, nil
}

func randomPIN(random io.Reader) (PIN, error) {
	var b [8]byte
	if _, err := io.ReadFull(random, b[:]); err != nil {
		return "", fmt.Errorf("make a one-time PIN: %w", err)
	}

	return PIN(fmt.Sprintf("%04d", binary.BigEndian.Uint64(b[:])%10000)), nil
}
