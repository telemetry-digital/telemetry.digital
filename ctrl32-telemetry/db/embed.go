// Package db embeds the SQL migrations so the single binary carries its own schema.
package db

import "embed"

// Migrations contains migrations/*.sql, applied in lexical order by internal/db.Migrate.
//
//go:embed migrations/*.sql
var Migrations embed.FS
