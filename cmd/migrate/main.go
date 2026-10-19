// Command migrate applies the schema as the owner role. The app never migrates.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/akkamaze/synqsong/db"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("migrate failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	url := os.Getenv("MIGRATION_DATABASE_URL")
	if url == "" {
		return errors.New("set MIGRATION_DATABASE_URL to the owner role's connection string")
	}

	conn, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("close database", "error", err)
		}
	}()

	goose.SetBaseFS(db.Migrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	if err := goose.UpContext(ctx, conn, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
