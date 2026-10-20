package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"synqsong/internal/account"
	"synqsong/internal/auth"
)

var _ auth.Store = (*Store)(nil)

// uniqueViolation is Postgres error 23505.
const uniqueViolation = "23505"

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
		return 0, auth.ErrUsernameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("save PIN login: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit account creation: %w", err)
	}

	return account.ID(id), nil
}

func (s *Store) FindPINLogin(ctx context.Context, usernameDigest []byte) (auth.PINLogin, bool, error) {
	var (
		login auth.PINLogin
		id    int64
	)

	err := s.pool.QueryRow(ctx, `
		SELECT account_id, username_digest, pin_salt, pin_digest, must_change_pin FROM pin_logins WHERE username_digest = $1`,
		usernameDigest).Scan(&id, &login.UsernameDigest, &login.Hash.Salt, &login.Hash.Digest, &login.MustChange)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.PINLogin{}, false, nil
	}
	if err != nil {
		return auth.PINLogin{}, false, fmt.Errorf("find PIN login: %w", err)
	}

	login.Account = account.ID(id)

	return login, true, nil
}

func (s *Store) PINOfAccount(ctx context.Context, id account.ID) (auth.PINLogin, error) {
	login := auth.PINLogin{Account: id}

	err := s.pool.QueryRow(ctx, `
		SELECT username_digest, pin_salt, pin_digest, must_change_pin FROM pin_logins WHERE account_id = $1`,
		int64(id)).Scan(&login.UsernameDigest, &login.Hash.Salt, &login.Hash.Digest, &login.MustChange)
	if err != nil {
		return auth.PINLogin{}, fmt.Errorf("read PIN of account %d: %w", id, err)
	}

	return login, nil
}

// ReplacePIN sets a new PIN and signs the account out everywhere, in one transaction.
// mustChange is true when an admin set it for them.
func (s *Store) ReplacePIN(ctx context.Context, id account.ID, pin auth.PINHash, mustChange bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin PIN change of account %d: %w", id, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE pin_logins SET pin_salt = $2, pin_digest = $3, must_change_pin = $4, updated_at = now()
		WHERE account_id = $1`, int64(id), pin.Salt, pin.Digest, mustChange)
	if err != nil {
		return fmt.Errorf("set PIN of account %d: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set PIN of account %d: no PIN login", id)
	}

	// Runs after the update so it also sees a session CreateSession committed while waiting on the row.
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE account_id = $1`, int64(id)); err != nil {
		return fmt.Errorf("sign out account %d everywhere: %w", id, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PIN change of account %d: %w", id, err)
	}

	return nil
}

func (s *Store) SiteRole(ctx context.Context, id account.ID) (account.SiteRole, error) {
	var role int16

	if err := s.pool.QueryRow(ctx, `SELECT site_role FROM accounts WHERE id = $1`, int64(id)).Scan(&role); err != nil {
		return 0, fmt.Errorf("read site role of account %d: %w", id, err)
	}

	return account.SiteRole(role), nil
}
