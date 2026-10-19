package auth

import (
	"context"
	"errors"

	"synqsong/internal/account"
)

// SignUp makes an account and signs it in. Every sign-up counts toward its address's limit,
// including one refused because the name is taken, which also caps probing for names.
func (s *Service) SignUp(ctx context.Context, from ClientAddr, rawUsername, rawPIN, rawName string) (Session, error) {
	username, err := ParseUsername(rawUsername)
	if err != nil {
		return Session{}, err
	}

	pin, err := ParsePIN(rawPIN)
	if err != nil {
		return Session{}, err
	}

	name, err := account.ParseDisplayName(rawName)
	if err != nil {
		return Session{}, err
	}

	attempt, err := s.limits.Reserve(s.now(), signUpAddrKey(from))
	if err != nil {
		return Session{}, err
	}
	defer attempt.Release()

	digest := s.keys.UsernameDigest(username)

	hash, err := s.pins.hash(ctx, pin, digest, s.random)
	if err != nil {
		return Session{}, err
	}

	id, err := s.store.CreatePINAccount(ctx, name, digest, hash)
	if errors.Is(err, ErrUsernameTaken) {
		attempt.Fail(s.now())

		return Session{}, err
	}
	if err != nil {
		return Session{}, err
	}

	attempt.Succeed(s.now())

	return s.startSession(ctx, Identity{Account: id}, digest, hash)
}

// SignIn checks a username and PIN from one client address. device is the browser's device
// token, or empty when it has none.
func (s *Service) SignIn(ctx context.Context, from ClientAddr, device DeviceToken, rawUsername, rawPIN string) (Session, error) {
	username, userErr := ParseUsername(rawUsername)
	pin, pinErr := ParsePIN(rawPIN)

	if userErr != nil || pinErr != nil {
		return Session{}, ErrWrongLogin
	}

	digest := s.keys.UsernameDigest(username)

	login, found, err := s.store.FindPINLogin(ctx, digest)
	if err != nil {
		return Session{}, err
	}

	attempt, err := s.limits.Reserve(s.now(), s.guessKeys(digest, login.Hash, device, from)...)
	if err != nil {
		return Session{}, err
	}
	defer attempt.Release()

	// An unknown name checks the zero hash, so it runs scrypt once, as a wrong PIN does.
	right, err := s.pins.verify(ctx, pin, digest, login.Hash)
	if err != nil {
		return Session{}, err
	}

	if !found || !right {
		attempt.Fail(s.now())

		return Session{}, ErrWrongLogin
	}

	attempt.Succeed(s.now())

	return s.startSession(ctx, Identity{Account: login.Account, MustChangePIN: login.MustChange}, digest, login.Hash)
}

// guessKeys are the limits a PIN check counts against: the device's own when it brings a valid
// token for this username and PIN, otherwise the username's, and the address's in both cases.
// An unknown username has the zero hash, which no token was issued for.
func (s *Service) guessKeys(usernameDigest []byte, pin PINHash, device DeviceToken, from ClientAddr) []limitKey {
	if id, ok := s.keys.deviceID(device, usernameDigest, pin, s.now()); ok {
		return []limitKey{deviceKey(usernameDigest, id), signInAddrKey(from)}
	}

	return []limitKey{usernameKey(usernameDigest), signInAddrKey(from)}
}

// startSession fails with ErrWrongLogin if the PIN changed after it was checked.
func (s *Service) startSession(ctx context.Context, who Identity, usernameDigest []byte, checked PINHash) (Session, error) {
	token, err := NewToken(s.random, who.Account)
	if err != nil {
		return Session{}, err
	}

	device, err := s.keys.newDeviceToken(usernameDigest, checked, s.now(), s.random)
	if err != nil {
		return Session{}, err
	}

	if err := s.store.CreateSession(ctx, s.keys.TokenDigest(token), who.Account, checked.Digest); err != nil {
		return Session{}, err
	}

	return Session{Token: token, Device: device, Identity: who}, nil
}
