#!/bin/sh
# Runs once, when the Postgres data directory is first created.
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
  -v owner_password="$SYNQSONG_OWNER_PASSWORD" \
  -v app_password="$SYNQSONG_APP_PASSWORD" <<'SQL'
-- A failed statement is logged with its text by default; keep passwords out.
SET log_min_error_statement = panic;

CREATE ROLE synqsong_owner LOGIN PASSWORD :'owner_password';
CREATE ROLE synqsong_app LOGIN PASSWORD :'app_password';

CREATE DATABASE synqsong OWNER synqsong_owner;
CREATE DATABASE synqsong_test OWNER synqsong_owner;

REVOKE ALL ON DATABASE synqsong, synqsong_test FROM PUBLIC;
GRANT CONNECT ON DATABASE synqsong, synqsong_test TO synqsong_owner, synqsong_app;
REVOKE CONNECT, TEMPORARY ON DATABASE postgres, template1 FROM PUBLIC;
SQL

for db in synqsong synqsong_test; do
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db" <<'SQL'
REVOKE ALL ON SCHEMA public FROM PUBLIC;
ALTER SCHEMA public OWNER TO synqsong_owner;
GRANT USAGE ON SCHEMA public TO synqsong_app;
SQL
done
