// Package db provides the PostgreSQL connection pool and the embedded migration runner.
package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	migrations "github.com/telemetry-digital/ctrl32-telemetry/db"
)

// Connect opens a pool and verifies the connection.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("db: parse url: %w", err)
	}
	// Eight connections by default; a larger installation sets pool_max_conns in the database URL
	// (…?pool_max_conns=24) after measuring PostgreSQL — every connection costs memory on the database server.
	if !strings.Contains(url, "pool_max_conns=") {
		cfg.MaxConns = 8
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// Migrate applies embedded migrations that are not yet recorded in public.schema_migration.
// Each migration file manages its own transaction (BEGIN/COMMIT), so the runner executes it verbatim.
// Returns the names of newly applied migrations.
func Migrate(ctx context.Context, adminURL string, appPassword string) ([]string, error) {
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return nil, fmt.Errorf("migrate: connect: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migration (
		name text PRIMARY KEY, sha256 text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("migrate: bootstrap: %w", err)
	}

	entries, err := fs.ReadDir(migrations.Migrations, "migrations")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var applied []string
	for _, name := range names {
		body, err := fs.ReadFile(migrations.Migrations, "migrations/"+name)
		if err != nil {
			return applied, err
		}
		sum := sha256.Sum256(body)
		hash := hex.EncodeToString(sum[:])

		var existing string
		err = conn.QueryRow(ctx, `SELECT sha256 FROM public.schema_migration WHERE name = $1`, name).Scan(&existing)
		switch {
		case err == nil:
			if existing != hash {
				return applied, fmt.Errorf("migrate: %s was modified after being applied (recorded %s, file %s) — migrations are immutable", name, existing[:12], hash[:12])
			}
			continue
		case err != pgx.ErrNoRows:
			return applied, fmt.Errorf("migrate: lookup %s: %w", name, err)
		}

		if _, err := conn.Exec(ctx, string(body)); err != nil {
			return applied, fmt.Errorf("migrate: apply %s: %w", name, err)
		}
		if _, err := conn.Exec(ctx, `INSERT INTO public.schema_migration (name, sha256) VALUES ($1, $2)`, name, hash); err != nil {
			return applied, fmt.Errorf("migrate: record %s: %w", name, err)
		}
		applied = append(applied, name)
	}

	if appPassword != "" {
		// Role name is fixed by the schema; the password is the only variable part.
		if _, err := conn.Exec(ctx, `ALTER ROLE ctrl32_app WITH LOGIN PASSWORD '`+strings.ReplaceAll(appPassword, "'", "''")+`'`); err != nil {
			return applied, fmt.Errorf("migrate: set app password: %w", err)
		}
	}
	return applied, nil
}
