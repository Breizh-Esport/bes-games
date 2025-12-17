package testutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// NewTestPool opens a pgx pool for tests using DATABASE_URL (or TEST_DATABASE_URL).
//
// You should provide a Postgres instance for tests (Docker is fine) and set one of:
// - TEST_DATABASE_URL (preferred for tests)
// - DATABASE_URL
//
// Example:
//
//	TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/bes_blind_test?sslmode=disable
func NewTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Fatalf("missing TEST_DATABASE_URL (or DATABASE_URL) for postgres-backed tests")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse postgres config: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open pgxpool: %v", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		t.Fatalf("ping postgres: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// RunMigrations runs goose migrations located under the repository's migrations directory.
//
// By default it uses:
// - backend/migrations
//
// You can override by setting:
// - BES_MIGRATIONS_DIR
func RunMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	dir := strings.TrimSpace(os.Getenv("BES_MIGRATIONS_DIR"))
	if dir == "" {
		dir = "backend/migrations"
	}

	// Make it robust when tests are executed from package directories.
	if !filepath.IsAbs(dir) {
		// Try relative to current working directory.
		dir = filepath.Clean(dir)
	}

	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose set dialect: %v", err)
	}

	if err := goose.UpContext(ctx, db, dir); err != nil {
		t.Fatalf("goose up (%s): %v", dir, err)
	}
}

// ResetSchema drops and recreates the public schema.
//
// This is useful to isolate tests without requiring a unique database per test.
// WARNING: This will delete all data in the database schema.
func ResetSchema(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO public;
`)
	if err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

// WithFreshDB resets schema and runs migrations, returning a ready pgx pool.
func WithFreshDB(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool := NewTestPool(ctx, t)
	ResetSchema(ctx, t, pool)
	RunMigrations(ctx, t, pool)
	return pool
}

// StdlibDB returns a *sql.DB backed by the given pgx pool (pgx stdlib adapter).
// This is occasionally useful for libraries that require database/sql.
func StdlibDB(pool *pgxpool.Pool) *sql.DB {
	return stdlib.OpenDBFromPool(pool)
}

// RequireEnv returns env var value or fails the test.
func RequireEnv(t *testing.T, key string) string {
	t.Helper()

	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		t.Fatalf("missing required env var %s", key)
	}
	return v
}

// Optional: best-effort helper for skipping postgres tests when not configured.
func SkipIfNoDB(t *testing.T) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Skip("postgres tests skipped: set TEST_DATABASE_URL (or DATABASE_URL)")
	}
}

// EnsureGooseVersionSanity is a tiny guard for cases where goose isn't linked correctly.
func EnsureGooseVersionSanity() error {
	// This is intentionally minimal; goose doesn't expose a version API.
	// If goose isn't imported properly, compilation will fail anyway.
	return nil
}

// WrapError provides a consistent error format for test helpers that return errors.
func WrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}
