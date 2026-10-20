//go:build integration

package storetest

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"mentorix-backend/internal/auth"
)

func TestAuthStore_RotateRefreshSession(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	userID := newTrainer(t, pool, "rotate@test.com", "")

	oldHash := randomTokenHash(t)
	expires := time.Now().UTC().Add(24 * time.Hour)
	if err := store.InsertRefreshSession(ctx, userID, oldHash, expires); err != nil {
		t.Fatalf("InsertRefreshSession() error = %v", err)
	}

	newHash := randomTokenHash(t)
	newExpires := time.Now().UTC().Add(30 * 24 * time.Hour)

	gotUserID, gotEmail, err := store.RotateRefreshSession(ctx, oldHash, newHash, newExpires)
	if err != nil {
		t.Fatalf("RotateRefreshSession() error = %v", err)
	}
	if gotUserID != userID {
		t.Errorf("user id = %v, want %v", gotUserID, userID)
	}
	if gotEmail != "rotate@test.com" {
		t.Errorf("email = %q, want rotate@test.com", gotEmail)
	}

	_, _, err = store.RotateRefreshSession(ctx, oldHash, newHash, newExpires)
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Errorf("rotate with revoked hash error = %v, want ErrInvalidRefresh", err)
	}
}

func TestAuthStore_RevokeRefreshSession_repeatKeepsFirstRevokedAt(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	userID := newTrainer(t, pool, "revoke-twice@test.com", "")
	revokedAt := func(tokenHash []byte) *time.Time {
		t.Helper()
		var got *time.Time
		if err := pool.QueryRow(ctx,
			`SELECT revoked_at FROM mentorix.auth_refresh_sessions WHERE token_hash = $1`, tokenHash,
		).Scan(&got); err != nil {
			t.Fatalf("read revoked_at: %v", err)
		}
		return got
	}

	tokenHash := randomTokenHash(t)
	if err := store.InsertRefreshSession(ctx, userID, tokenHash, time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatalf("InsertRefreshSession: %v", err)
	}
	if err := store.RevokeRefreshSession(ctx, tokenHash); err != nil {
		t.Fatalf("first RevokeRefreshSession: %v", err)
	}
	first := revokedAt(tokenHash)
	if first == nil {
		t.Fatal("revoked_at is NULL after the first revoke")
	}

	time.Sleep(20 * time.Millisecond)
	if err := store.RevokeRefreshSession(ctx, tokenHash); err != nil {
		t.Fatalf("second RevokeRefreshSession returned an error: %v", err)
	}
	second := revokedAt(tokenHash)
	if second == nil || !second.Equal(*first) {
		t.Errorf("revoked_at after repeat revoke = %v, want unchanged %v", second, first)
	}
}

func TestAuthStore_RotateRefreshSession_revokesOldKeepsNewActive(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	userID := newTrainer(t, pool, "rotate-revokes@test.com", "")
	oldHash, newHash := randomTokenHash(t), randomTokenHash(t)
	if err := store.InsertRefreshSession(ctx, userID, oldHash, time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatalf("InsertRefreshSession: %v", err)
	}
	if _, _, err := store.RotateRefreshSession(ctx, oldHash, newHash, time.Now().UTC().Add(48*time.Hour)); err != nil {
		t.Fatalf("RotateRefreshSession: %v", err)
	}

	var oldRevoked, newRevoked *time.Time
	q := `SELECT revoked_at FROM mentorix.auth_refresh_sessions WHERE token_hash = $1`
	if err := pool.QueryRow(ctx, q, oldHash).Scan(&oldRevoked); err != nil {
		t.Fatalf("read old revoked_at: %v", err)
	}
	if err := pool.QueryRow(ctx, q, newHash).Scan(&newRevoked); err != nil {
		t.Fatalf("read new revoked_at: %v", err)
	}
	if oldRevoked == nil {
		t.Error("old session revoked_at is NULL after rotation")
	}
	if newRevoked != nil {
		t.Errorf("new session revoked_at = %v, want NULL", newRevoked)
	}
}

func randomTokenHash(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return b
}
