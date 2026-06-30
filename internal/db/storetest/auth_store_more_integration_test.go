//go:build integration

package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/auth"
)

func TestAuthStore_RevokeAndGrantRole(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.RegisterTrainerEmailPassword(ctx, "grant@test.com", hash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	tokenHash := randomTokenHash(t)
	if err := store.InsertRefreshSession(ctx, userID, tokenHash, mustFuture(t)); err != nil {
		t.Fatalf("InsertRefreshSession: %v", err)
	}
	if err := store.RevokeRefreshSession(ctx, tokenHash); err != nil {
		t.Fatalf("RevokeRefreshSession: %v", err)
	}
	if err := store.RevokeAllUserRefreshSessions(ctx, userID); err != nil {
		t.Fatalf("RevokeAllUserRefreshSessions: %v", err)
	}
	if err := store.GrantRole(ctx, userID, auth.RoleAdmin); err != nil {
		t.Fatalf("GrantRole: %v", err)
	}
	roles, err := store.UserRoles(ctx, userID)
	if err != nil {
		t.Fatalf("UserRoles: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("roles = %v, want trainer+admin", roles)
	}
}

func mustFuture(t *testing.T) time.Time {
	t.Helper()
	return time.Now().UTC().Add(24 * time.Hour)
}

func TestAuthStore_UpdateUserDisplayName(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.RegisterTrainerEmailPassword(ctx, "display-name@test.com", hash, "Before")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := store.UpdateUserDisplayName(ctx, userID, "After"); err != nil {
		t.Fatalf("UpdateUserDisplayName: %v", err)
	}
	profile, err := store.UserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("UserProfile: %v", err)
	}
	if profile.Name != "After" {
		t.Fatalf("name = %q, want After", profile.Name)
	}
}

func TestAuthStore_UserProfileNotFound(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	_, err := store.UserProfile(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestAuthStore_UpdateUserDisplayNameNotFound(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	err := store.UpdateUserDisplayName(context.Background(), uuid.New(), "Nobody")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestAuthStore_UserPrimaryEmailNotFound(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	_, err := store.UserPrimaryEmail(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestAuthStore_UserRoles_emptyForUnknownUser(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	roles, err := store.UserRoles(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("UserRoles: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("roles = %v, want empty", roles)
	}
}
