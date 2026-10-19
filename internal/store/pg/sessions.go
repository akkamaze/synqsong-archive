package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"synqsong/internal/account"
	"synqsong/internal/auth"
)

// CreateSession signs an account in only if its PIN is still the one sign-in checked, so a
// sign-in racing a PIN change fails instead of leaving a session behind. FOR SHARE waits for
// an uncommitted PIN change and then rechecks the digest against the committed row.
func (s *Store) CreateSession(ctx context.Context, tokenDigest []byte, id account.ID, checkedPIN []byte) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (token_digest, account_id)
		SELECT $1, account_id FROM pin_logins WHERE account_id = $2 AND pin_digest = $3
		FOR SHARE`, tokenDigest, int64(id), checkedPIN)
	if err != nil {
		return fmt.Errorf("create session for account %d: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return auth.ErrWrongLogin
	}

	return nil
}

// SessionAccount returns who a session belongs to if it was used within idle, by the database's
// clock. It marks the session used at most once an hour, so reading a session rarely writes.
func (s *Store) SessionAccount(ctx context.Context, tokenDigest []byte, idle time.Duration) (auth.Identity, bool, error) {
	var (
		id         int64
		mustChange *bool
	)

	err := s.pool.QueryRow(ctx, `
		WITH live AS (
			SELECT account_id FROM sessions
			WHERE token_digest = $1 AND last_used_at > now() - $2::interval
		), touched AS (
			UPDATE sessions SET last_used_at = now()
			WHERE token_digest = $1 AND last_used_at > now() - $2::interval AND last_used_at < now() - interval '1 hour'
		)
		SELECT live.account_id, p.must_change_pin
		FROM live LEFT JOIN pin_logins p ON p.account_id = live.account_id`, tokenDigest, idle).Scan(&id, &mustChange)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Identity{}, false, nil
	}
	if err != nil {
		return auth.Identity{}, false, fmt.Errorf("look up session: %w", err)
	}

	return auth.Identity{Account: account.ID(id), MustChangePIN: mustChange != nil && *mustChange}, true, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenDigest []byte) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_digest = $1`, tokenDigest); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// DeleteExpiredSessions removes sessions unused for longer than idle and says how many went.
func (s *Store) DeleteExpiredSessions(ctx context.Context, idle time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE last_used_at <= now() - $1::interval`, idle)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}

	return tag.RowsAffected(), nil
}
