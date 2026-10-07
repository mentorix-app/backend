//go:build integration

package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/storetest"
)

func TestStore_busyFamilyAnswersErrRefreshBusy(t *testing.T) {
	pool := storetest.NewPool(t)
	ctx := context.Background()
	store := auth.NewStore(pool)
	store.SetLockTimeoutForTest(200 * time.Millisecond)

	userID, err := store.RegisterTrainerEmailPassword(ctx, "busy@test.com", "hash", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	tokenHash := []byte("busy-test-token-hash-0123456789ab")
	if err := store.InsertRefreshSession(ctx, userID, tokenHash, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var family uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT family_id FROM mentorix.auth_refresh_sessions WHERE token_hash = $1`, tokenHash).Scan(&family); err != nil {
		t.Fatalf("read family: %v", err)
	}

	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, family.String()); err != nil {
		t.Fatalf("hold family lock: %v", err)
	}

	snapshot := func() string {
		var s string
		if err := pool.QueryRow(ctx,
			`SELECT count(*)::text || '|' || count(revoked_at)::text || '|' || count(rotated_at)::text
			 FROM mentorix.auth_refresh_sessions`).Scan(&s); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		return s
	}
	before := snapshot()

	calls := map[string]func() error{
		"rotate": func() error {
			_, err := store.RotateRefreshSession(ctx, tokenHash, []byte("busy-test-new-hash-0123456789abcd"), time.Now().UTC().Add(time.Hour), auth.DefaultRefreshReuseGrace)
			return err
		},
		"logout":     func() error { return store.RevokeRefreshSession(ctx, tokenHash) },
		"logout-all": func() error { return store.RevokeAllUserRefreshSessions(ctx, userID) },
	}
	for name, call := range calls {
		started := time.Now()
		err := call()
		if !errors.Is(err, auth.ErrRefreshBusy) {
			t.Errorf("%s error = %v, want ErrRefreshBusy", name, err)
		}
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			t.Errorf("%s waited %v, want about the lock timeout", name, elapsed)
		}
	}
	if after := snapshot(); after != before {
		t.Errorf("rows changed while busy: %s, want %s", after, before)
	}
}

// A row lock held by another connection makes a statement after the advisory lock
// wait. lock_timeout covers it, so that statement maps to ErrRefreshBusy too.
func TestStore_busyRowLockAnswersErrRefreshBusy(t *testing.T) {
	pool := storetest.NewPool(t)
	ctx := context.Background()
	store := auth.NewStore(pool)
	store.SetLockTimeoutForTest(200 * time.Millisecond)

	userID, err := store.RegisterTrainerEmailPassword(ctx, "busy-row@test.com", "hash", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	expires := time.Now().UTC().Add(time.Hour)
	mine := []byte("busy-row-test-token-0123456789abc")
	other := []byte("busy-row-other-token-0123456789ab")
	for _, h := range [][]byte{mine, other} {
		if err := store.InsertRefreshSession(ctx, userID, h, expires); err != nil {
			t.Fatalf("insert session: %v", err)
		}
	}

	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var held []byte
	if err := tx.QueryRow(ctx,
		`SELECT token_hash FROM mentorix.auth_refresh_sessions WHERE token_hash = $1 FOR UPDATE`, other).Scan(&held); err != nil {
		t.Fatalf("hold row lock: %v", err)
	}

	if err := store.RevokeAllUserRefreshSessions(ctx, userID); !errors.Is(err, auth.ErrRefreshBusy) {
		t.Errorf("logout-all error = %v, want ErrRefreshBusy", err)
	}
	var revoked int
	if err := pool.QueryRow(ctx,
		`SELECT count(revoked_at) FROM mentorix.auth_refresh_sessions WHERE user_id = $1`, userID).Scan(&revoked); err != nil {
		t.Fatalf("count revoked: %v", err)
	}
	if revoked != 0 {
		t.Errorf("%d rows revoked by a timed out logout-all, want 0", revoked)
	}
}
