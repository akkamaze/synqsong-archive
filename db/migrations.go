// Package db holds the schema migrations, embedded so the migrate binary carries them.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
