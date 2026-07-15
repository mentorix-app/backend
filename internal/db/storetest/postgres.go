//go:build integration

package storetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

var testDBMu sync.Mutex

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func resolveTestDatabaseURL(t *testing.T) string {
	t.Helper()
	_ = godotenv.Load(filepath.Join(repoRoot(t), ".env"))

	if u := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")); u != "" {
		return u
	}
	u := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if u == "" {
		t.Skip("TEST_DATABASE_URL (or DATABASE_URL with mentorix_test) is not set")
	}
	if !strings.Contains(u, "mentorix_test") {
		t.Fatalf("refusing DATABASE_URL %q: use a mentorix_test database or set TEST_DATABASE_URL", u)
	}
	return u
}

// NewPoolWithURL resets mentorix schema on the test database, runs migrations, and returns a pool and URL.
func NewPoolWithURL(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	testDBMu.Lock()
	t.Cleanup(testDBMu.Unlock)

	connStr := resolveTestDatabaseURL(t)
	ctx := context.Background()

	bootstrap, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool bootstrap: %v", err)
	}
	defer bootstrap.Close()

	if _, err := bootstrap.Exec(ctx, `DROP SCHEMA IF EXISTS mentorix CASCADE`); err != nil {
		t.Fatalf("drop schema mentorix: %v", err)
	}
	if _, err := bootstrap.Exec(ctx, `DROP TABLE IF EXISTS public.schema_migrations`); err != nil {
		t.Fatalf("drop schema_migrations: %v", err)
	}

	migrationsPath := filepath.Join(repoRoot(t), "db", "migrations")
	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		connStr,
	)
	if err != nil {
		t.Fatalf("migrate new: %v", err)
	}
	t.Cleanup(func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			t.Logf("migrate close source: %v", srcErr)
		}
		if dbErr != nil {
			t.Logf("migrate close db: %v", dbErr)
		}
	})

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool new: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool, connStr
}

// NewPool resets the test database, runs migrations, and returns a pool.
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := NewPoolWithURL(t)
	return pool
}
