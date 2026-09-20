package sqlite

import "embed"

// migrationFS contains the native flow schema migrations.
//
//go:embed migrations/*.sql
var embeddedMigrations embed.FS
