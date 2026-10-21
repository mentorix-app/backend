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

	userID := newTrainer(t, pool, "grant@test.com", "")

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
	promoteToAdminOnly(t, pool, userID)
	roles, err := store.UserRoles(ctx, userID)
	if err != nil {
		t.Fatalf("UserRoles: %v", err)
	}
	if len(roles) != 1 || roles[0] != auth.RoleAdmin {
		t.Fatalf("roles = %v, want [admin]", roles)
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

	userID := newTrainer(t, pool, "display-name@test.com", "Before")
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

// A trainer-only account is the legacy shape from before the app became client-first: it
// has the trainer role and no client role, and no Telegram sign-in.
func TestAuthStore_UserProfile_trainerOnlyAccount(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)

	userID := newTrainer(t, pool, "profile-trainer@test.com", "Profile Coach")

	profile, err := store.UserProfile(context.Background(), userID)
	if err != nil {
		t.Fatalf("UserProfile: %v", err)
	}
	if len(profile.Roles) != 1 || profile.Roles[0] != auth.RoleTrainer {
		t.Fatalf("roles = %v, want [trainer]", profile.Roles)
	}
	if profile.Email != "profile-trainer@test.com" {
		t.Fatalf("email = %q, want %q", profile.Email, "profile-trainer@test.com")
	}
	if profile.Name != "Profile Coach" {
		t.Fatalf("name = %q, want %q", profile.Name, "Profile Coach")
	}
	if profile.TelegramLinked {
		t.Fatal("telegram linked = true, want false")
	}
}
