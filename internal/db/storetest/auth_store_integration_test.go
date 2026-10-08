//go:build integration

package storetest

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/auth"
)

func TestAuthStore_RegisterTrainerEmailPassword(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	email := "trainer-integration@test.com"
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	userID, err := store.RegisterTrainerEmailPassword(ctx, email, hash, "Viktor")
	if err != nil {
		t.Fatalf("RegisterTrainerEmailPassword() error = %v", err)
	}
	if userID == uuid.Nil {
		t.Fatal("user id is nil")
	}

	profile, err := store.UserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != email {
		t.Errorf("email = %q, want %q", profile.Email, email)
	}
	if len(profile.Roles) != 1 || profile.Roles[0] != auth.RoleTrainer {
		t.Errorf("roles = %v, want [%q]", profile.Roles, auth.RoleTrainer)
	}

	primary, err := store.UserPrimaryEmail(ctx, userID)
	if err != nil {
		t.Fatalf("UserPrimaryEmail() error = %v", err)
	}
	if primary != email {
		t.Errorf("primary email = %q, want %q", primary, email)
	}

	if profile.Name != "Viktor" {
		t.Errorf("name = %q, want %q", profile.Name, "Viktor")
	}

	_, err = store.RegisterTrainerEmailPassword(ctx, email, hash, "")
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Errorf("duplicate register error = %v, want ErrEmailTaken", err)
	}
}

func TestAuthStore_RotateRefreshSession(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.RegisterTrainerEmailPassword(ctx, "rotate@test.com", hash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	oldHash := randomTokenHash(t)
	expires := time.Now().UTC().Add(24 * time.Hour)
	if err := store.InsertRefreshSession(ctx, userID, oldHash, expires); err != nil {
		t.Fatalf("InsertRefreshSession() error = %v", err)
	}

	newHash := randomTokenHash(t)
	newExpires := time.Now().UTC().Add(30 * 24 * time.Hour)

	gotUserID, err := store.RotateRefreshSession(ctx, oldHash, newHash, newExpires)
	if err != nil {
		t.Fatalf("RotateRefreshSession() error = %v", err)
	}
	if gotUserID != userID {
		t.Errorf("user id = %v, want %v", gotUserID, userID)
	}

	_, err = store.RotateRefreshSession(ctx, oldHash, newHash, newExpires)
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Errorf("rotate with revoked hash error = %v, want ErrInvalidRefresh", err)
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
