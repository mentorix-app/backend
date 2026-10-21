//go:build integration

package storetest

import (
	"context"
	"testing"
	"time"

	"mentorix-backend/internal/auth"
)

func TestAuthService_SignInRefreshLogout(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	const jwtSecret = "integration-test-jwt-secret-32chars"
	email := "auth-svc@test.com"
	verifier := scriptedVerifier{claims: auth.IDTokenClaims{
		Subject:       "google-auth-svc",
		Email:         email,
		EmailVerified: true,
		Name:          "Auth User",
	}}
	svc := auth.NewService(pool, jwtSecret, 15*time.Minute, 30*24*time.Hour, auth.WithGoogleVerifier(verifier))

	signedIn, err := svc.SignInWithGoogle(ctx, "raw", "")
	if err != nil {
		t.Fatalf("SignInWithGoogle() error = %v", err)
	}
	if signedIn.AccessToken == "" || signedIn.RefreshToken == "" {
		t.Fatal("expected tokens from sign-in")
	}

	refreshed, err := svc.Refresh(ctx, signedIn.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshed.RefreshToken == signedIn.RefreshToken {
		t.Fatal("expected rotated refresh token")
	}

	if err := svc.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if err := svc.LogoutAll(ctx, signedIn.UserID); err != nil {
		t.Fatalf("LogoutAll() error = %v", err)
	}

	profile, err := svc.UserProfile(ctx, signedIn.UserID)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != email {
		t.Errorf("email = %q, want %q", profile.Email, email)
	}
	if profile.Name != "Auth User" {
		t.Errorf("name = %q, want %q", profile.Name, "Auth User")
	}

	updated, err := svc.UpdateProfileName(ctx, signedIn.UserID, "Renamed")
	if err != nil {
		t.Fatalf("UpdateProfileName() error = %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("updated name = %q, want %q", updated.Name, "Renamed")
	}
}
