package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all embedded SQL migrations in lexical order exactly once,
// tracking applied versions in schema_migrations. Each file runs in its own
// transaction so a failure rolls back cleanly.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (applied []string, err error) {
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version,
		).Scan(&exists); err != nil {
			return applied, fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			continue
		}
		body, rerr := migrationsFS.ReadFile("migrations/" + name)
		if rerr != nil {
			return applied, fmt.Errorf("read migration %s: %w", name, rerr)
		}
		tx, berr := pool.Begin(ctx)
		if berr != nil {
			return applied, berr
		}
		if _, eerr := tx.Exec(ctx, string(body)); eerr != nil {
			_ = tx.Rollback(ctx)
			return applied, fmt.Errorf("apply migration %s: %w", name, eerr)
		}
		if _, eerr := tx.Exec(ctx,
			`INSERT INTO schema_migrations(version) VALUES($1)`, version); eerr != nil {
			_ = tx.Rollback(ctx)
			return applied, fmt.Errorf("record migration %s: %w", name, eerr)
		}
		if cerr := tx.Commit(ctx); cerr != nil {
			return applied, fmt.Errorf("commit migration %s: %w", name, cerr)
		}
		applied = append(applied, version)
	}
	return applied, nil
}
