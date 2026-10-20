package pg

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// insufficientPrivilege is Postgres error 42501.
const insufficientPrivilege = "42501"

func TestTheAppCannotRewriteRolesOrWhoALoginBelongsTo(t *testing.T) {
	needDB(t)

	refused := []string{
		`UPDATE accounts SET site_role = 2 WHERE false`,
		`UPDATE accounts SET display_name = 'x' WHERE false`,
		`UPDATE pin_logins SET username_digest = username_digest WHERE false`,
		`UPDATE pin_logins SET account_id = account_id WHERE false`,
		`INSERT INTO accounts (site_role) VALUES (2)`,
		`INSERT INTO accounts (display_name, site_role) VALUES ('Mallory', 2)`,
		`INSERT INTO accounts (created_at) VALUES (now())`,
	}

	for _, stmt := range refused {
		_, err := testPool.Exec(context.Background(), stmt)

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != insufficientPrivilege {
			t.Errorf("%s as the app: %v, want permission denied", stmt, err)
		}
	}

	if _, err := testPool.Exec(context.Background(), `
		UPDATE pin_logins SET pin_salt = pin_salt, pin_digest = pin_digest, must_change_pin = must_change_pin, updated_at = updated_at
		WHERE false`); err != nil {
		t.Fatalf("changing a PIN as the app: %v, want allowed", err)
	}

	var role int16
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO accounts (display_name) VALUES ('Ann') RETURNING site_role`).Scan(&role); err != nil || role != 1 {
		t.Fatalf("adding an account as the app = role %d, %v; want allowed as a site user", role, err)
	}
}
