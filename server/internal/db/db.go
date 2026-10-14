// Package db wires the Postgres connection pool and runs golang-migrate
// migrations on startup. Migrations are embedded into the binary so deployment
// stays "one binary + one Postgres container".
package db

import (
	"context"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect opens a pgx connection pool to the given Postgres URL.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

// Migrate runs all up migrations against the given database URL.
func Migrate(databaseURL string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgxURL(databaseURL))
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// MigrateTo migrates the database to exactly the given schema version (up or
// down). Tests use it to seed data under an older schema and then assert the
// rows survive the remaining up migrations.
func MigrateTo(databaseURL string, version uint) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgxURL(databaseURL))
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()
	if err := m.Migrate(version); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate to %d: %w", version, err)
	}
	return nil
}

// pgxURL ensures the scheme is the one the pgx/v5 migrate driver registers for.
func pgxURL(databaseURL string) string {
	// The pgx/v5 driver registers under "pgx5". Rewrite the common
	// postgres://|postgresql:// schemes to it.
	for _, prefix := range []string{"postgresql://", "postgres://"} {
		if len(databaseURL) >= len(prefix) && databaseURL[:len(prefix)] == prefix {
			return "pgx5://" + databaseURL[len(prefix):]
		}
	}
	return databaseURL
}

// ensure the pgx driver package is linked (registers the "pgx5" driver).
var _ = pgx.Postgres{}
