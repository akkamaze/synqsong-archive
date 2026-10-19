-- +goose Up

-- Site roles 1 user, 2 admin. An admin can set someone a new PIN.
ALTER TABLE accounts
    ADD COLUMN display_name text NOT NULL DEFAULT '' CHECK (char_length(display_name) <= 40),
    ADD COLUMN site_role    smallint NOT NULL DEFAULT 1 CHECK (site_role IN (1, 2));

-- Sign-in by username and PIN. Neither is stored: the username only as an HMAC for lookup,
-- the PIN as HMAC(key, scrypt(pin, salt)), with the key outside the database.
CREATE TABLE pin_logins (
    account_id      bigint PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    username_digest bytea NOT NULL UNIQUE CHECK (octet_length(username_digest) = 32),
    pin_salt        text NOT NULL CHECK (pin_salt ~ '^[0-9a-f]{32}$'),
    pin_digest      bytea NOT NULL CHECK (octet_length(pin_digest) = 32),
    must_change_pin boolean NOT NULL DEFAULT false,
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Only a hash of each session token is kept, so a stolen database cannot sign anyone in.
CREATE TABLE sessions (
    token_digest bytea PRIMARY KEY CHECK (octet_length(token_digest) = 32),
    account_id   bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_account ON sessions (account_id);

-- The app may add an account and change a PIN, but never set a site role or whose login it is.
REVOKE INSERT ON accounts FROM synqsong_app;
GRANT INSERT (display_name) ON accounts TO synqsong_app;
GRANT SELECT, INSERT ON pin_logins TO synqsong_app;
GRANT UPDATE (pin_salt, pin_digest, must_change_pin, updated_at) ON pin_logins TO synqsong_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON sessions TO synqsong_app;

-- +goose Down

GRANT INSERT ON accounts TO synqsong_app;
DROP TABLE sessions, pin_logins;
ALTER TABLE accounts DROP COLUMN site_role, DROP COLUMN display_name;
