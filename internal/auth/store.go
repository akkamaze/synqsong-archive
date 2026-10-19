package auth

import (
	"context"
	"errors"
	"time"

	"synqsong/internal/account"
)

// Store keeps PIN logins and sessions. It never sees a username, PIN or token, only their digests.
type Store interface {
	CreatePINAccount(ctx context.Context, name account.DisplayName, usernameDigest []byte, pin PINHash) (account.ID, error)
	FindPINLogin(ctx context.Context, usernameDigest []byte) (PINLogin, bool, error)
	PINOfAccount(ctx context.Context, id account.ID) (PINLogin, error)
	// ReplacePIN sets a new PIN and deletes the account's sessions in one transaction.
	ReplacePIN(ctx context.Context, id account.ID, pin PINHash, mustChange bool) error
	SiteRole(ctx context.Context, id account.ID) (account.SiteRole, error)
	// CreateSession stores a session only while checkedPIN is still the account's PIN digest,
	// and returns ErrWrongLogin otherwise.
	CreateSession(ctx context.Context, tokenDigest []byte, id account.ID, checkedPIN []byte) error
	// SessionAccount returns who a session belongs to if it was used within idle, and marks it used.
	SessionAccount(ctx context.Context, tokenDigest []byte, idle time.Duration) (Identity, bool, error)
	DeleteSession(ctx context.Context, tokenDigest []byte) error
	DeleteExpiredSessions(ctx context.Context, idle time.Duration) (int64, error)
}

// PINLogin is what sign-in needs to check a PIN for one username.
type PINLogin struct {
	Account        account.ID
	UsernameDigest []byte
	Hash           PINHash
	MustChange     bool
}

// Identity is who a session belongs to. MustChangePIN means only changing the PIN is allowed.
type Identity struct {
	Account       account.ID
	MustChangePIN bool
}

var ErrUsernameTaken = errors.New("that username is taken, choose another")
