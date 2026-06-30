//go:build integration

package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/auth"
)

func TestAuthService_RegisterLoginRefreshLogout(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	const jwtSecret = "integration-test-jwt-secret-32chars"
	svc := auth.NewService(pool, jwtSecret, 15*time.Minute, 30*24*time.Hour)

	email := "auth-svc@test.com"
	password := "password123"

	registered, err := svc.RegisterTrainer(ctx, email, password, "Auth User")
	if err != nil {
		t.Fatalf("RegisterTrainer() error = %v", err)
	}
	if registered.AccessToken == "" || registered.RefreshToken == "" {
		t.Fatal("expected tokens from register")
	}

	loggedIn, err := svc.Login(ctx, email, password)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loggedIn.UserID != registered.UserID {
		t.Errorf("login user id = %v, want %v", loggedIn.UserID, registered.UserID)
	}

	refreshed, err := svc.Refresh(ctx, loggedIn.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshed.RefreshToken == loggedIn.RefreshToken {
		t.Fatal("expected rotated refresh token")
	}

	if err := svc.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if err := svc.LogoutAll(ctx, registered.UserID); err != nil {
		t.Fatalf("LogoutAll() error = %v", err)
	}

	profile, err := svc.UserProfile(ctx, registered.UserID)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != email {
		t.Errorf("email = %q, want %q", profile.Email, email)
	}
	if profile.Name != "Auth User" {
		t.Errorf("name = %q, want %q", profile.Name, "Auth User")
	}

	updated, err := svc.UpdateProfileName(ctx, registered.UserID, "Renamed")
	if err != nil {
		t.Fatalf("UpdateProfileName() error = %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("updated name = %q, want %q", updated.Name, "Renamed")
	}
}

func TestAuthService_LoginWrongPassword(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	const jwtSecret = "integration-test-jwt-secret-32chars"
	svc := auth.NewService(pool, jwtSecret, 15*time.Minute, 30*24*time.Hour)

	email := "wrong-pw@test.com"
	if _, err := svc.RegisterTrainer(ctx, email, "password123", ""); err != nil {
		t.Fatalf("RegisterTrainer() error = %v", err)
	}
	if _, err := svc.Login(ctx, email, "wrong-password"); err == nil {
		t.Fatal("expected login error for wrong password")
	}
}

func TestAuthService_UserPrimaryEmailNotFound(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	svc := auth.NewService(pool, "integration-test-jwt-secret-32chars", 15*time.Minute, 30*24*time.Hour)

	_, err := svc.UserPrimaryEmail(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected not found error")
	}
}
