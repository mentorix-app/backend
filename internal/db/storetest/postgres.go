//go:build integration

package storetest

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// NewPoolWithURL starts ephemeral Postgres, runs db/migrations, and returns a pool and connection URL.
func NewPoolWithURL(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	requireDocker(t)

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("mentorix_test"),
		postgres.WithUsername("mentorix"),
		postgres.WithPassword("mentorix"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		if termErr := pgContainer.Terminate(context.Background()); termErr != nil {
			t.Logf("terminate postgres container: %v", termErr)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
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

// NewPool starts ephemeral Postgres, runs db/migrations, and returns a pool.
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := NewPoolWithURL(t)
	return pool
}

func requireDocker(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		t.Skipf("docker not available for integration tests: %v", err)
	}
}
