package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"synqsong/internal/account"
	"synqsong/internal/auth"
)

var ErrUsernameTaken = errors.New("that username is taken")

// uniqueViolation is Postgres error 23505.
const uniqueViolation = "23505"

// PINLogin is what sign-in needs to check a PIN for one username.
type PINLogin struct {
	Account    account.ID
	Hash       auth.PINHash
	MustChange bool
}

type Profile struct {
	Name account.DisplayName
	Role account.SiteRole
}

// CreatePINAccount makes an account that signs in with a username and PIN, in one transaction.
func (s *Store) CreatePINAccount(ctx context.Context, name account.DisplayName, usernameDigest []byte, pin auth.PINHash) (account.ID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin account creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO accounts (display_name) VALUES ($1) RETURNING id`, string(name)).Scan(&id); err != nil {
		return 0, fmt.Errorf("create account: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO pin_logins (account_id, username_digest, pin_salt, pin_digest) VALUES ($1, $2, $3, $4)`,
		id, usernameDigest, pin.Salt, pin.Digest)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return 0, ErrUsernameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("save PIN login: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit account creation: %w", err)
	}

	return account.ID(id), nil
}

func (s *Store) FindPINLogin(ctx context.Context, usernameDigest []byte) (PINLogin, bool, error) {
	var (
		login PINLogin
		id    int64
	)

	err := s.pool.QueryRow(ctx, `
		SELECT account_id, pin_salt, pin_digest, must_change_pin FROM pin_logins WHERE username_digest = $1`,
		usernameDigest).Scan(&id, &login.Hash.Salt, &login.Hash.Digest, &login.MustChange)
	if errors.Is(err, pgx.ErrNoRows) {
		return PINLogin{}, false, nil
	}
	if err != nil {
		return PINLogin{}, false, fmt.Errorf("find PIN login: %w", err)
	}

	login.Account = account.ID(id)

	return login, true, nil
}

// SetPIN replaces an account's PIN. mustChange is true when an admin set it for them.
func (s *Store) SetPIN(ctx context.Context, id account.ID, pin auth.PINHash, mustChange bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE pin_logins SET pin_salt = $2, pin_digest = $3, must_change_pin = $4, updated_at = now()
		WHERE account_id = $1`, int64(id), pin.Salt, pin.Digest, mustChange)
	if err != nil {
		return fmt.Errorf("set PIN of account %d: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set PIN of account %d: no PIN login", id)
	}

	return nil
}

func (s *Store) Profile(ctx context.Context, id account.ID) (Profile, error) {
	var (
		name string
		role int16
	)

	err := s.pool.QueryRow(ctx, `SELECT display_name, site_role FROM accounts WHERE id = $1`, int64(id)).Scan(&name, &role)
	if err != nil {
		return Profile{}, fmt.Errorf("read profile of account %d: %w", id, err)
	}

	return Profile{Name: account.DisplayName(name), Role: account.SiteRole(role)}, nil
}

func (s *Store) CreateSession(ctx context.Context, tokenDigest []byte, id account.ID) error {
	if _, err := s.pool.Exec(ctx, `INSERT INTO sessions (token_digest, account_id) VALUES ($1, $2)`, tokenDigest, int64(id)); err != nil {
		return fmt.Errorf("create session for account %d: %w", id, err)
	}

	return nil
}

// SessionAccount returns who a session belongs to if it was used after usedAfter, and marks it used now.
func (s *Store) SessionAccount(ctx context.Context, tokenDigest []byte, usedAfter time.Time) (account.ID, bool, error) {
	var id int64

	err := s.pool.QueryRow(ctx, `
		UPDATE sessions SET last_used_at = now()
		WHERE token_digest = $1 AND last_used_at > $2
		RETURNING account_id`, tokenDigest, usedAfter).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("look up session: %w", err)
	}

	return account.ID(id), true, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenDigest []byte) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_digest = $1`, tokenDigest); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// DeleteSessionsOf signs an account out everywhere, as after an admin sets a new PIN.
func (s *Store) DeleteSessionsOf(ctx context.Context, id account.ID) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE account_id = $1`, int64(id)); err != nil {
		return fmt.Errorf("delete sessions of account %d: %w", id, err)
	}

	return nil
}
