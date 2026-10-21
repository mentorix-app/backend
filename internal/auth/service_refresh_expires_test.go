package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestService_RefreshExpiresMatchesStoredExpiry(t *testing.T) {
	verifier := fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "sub", Email: "a@b.com", EmailVerified: true, Nonce: hashedNonce(testAppleNonce)}}
	tests := []struct {
		name string
		// issue returns the session and the expiry the store received.
		issue func(t *testing.T) (IssuedAuth, time.Time)
	}{
		{"google", func(t *testing.T) (IssuedAuth, time.Time) {
			store := &fakeAuthStore{profile: UserProfile{Email: "a@b.com"}}
			issued, err := testGoogleService(store, verifier).SignInWithGoogle(context.Background(), "raw", "")
			if err != nil {
				t.Fatalf("SignInWithGoogle() error = %v", err)
			}
			return issued, store.insertedExpiry
		}},
		{"apple", func(t *testing.T) (IssuedAuth, time.Time) {
			store := &fakeAuthStore{profile: UserProfile{Email: "a@b.com"}}
			issued, err := testAppleService(store, verifier).SignInWithApple(context.Background(), "raw", "", testAppleNonce)
			if err != nil {
				t.Fatalf("SignInWithApple() error = %v", err)
			}
			return issued, store.insertedExpiry
		}},
		{"telegram link", func(t *testing.T) (IssuedAuth, time.Time) {
			store := &fakeAuthStore{linkRemaining: uuid.New(), profile: UserProfile{Email: "a@b.com"}}
			svc := testLinkService(store, &fakeLinkCodes{telegramUserID: "tg-1", ok: true})
			issued, err := svc.LinkTelegram(context.Background(), uuid.New(), "ABCD2345")
			if err != nil {
				t.Fatalf("LinkTelegram() error = %v", err)
			}
			return issued, store.insertedExpiry
		}},
		{"refresh", func(t *testing.T) (IssuedAuth, time.Time) {
			store := &fakeAuthStore{}
			issued, err := testAuthService(store).Refresh(context.Background(), "refresh-plain-token")
			if err != nil {
				t.Fatalf("Refresh() error = %v", err)
			}
			return issued, store.rotatedExpiry
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			issued, stored := tt.issue(t)
			after := time.Now()

			if stored.IsZero() {
				t.Fatal("store received no expiry")
			}
			if !issued.RefreshExpires.Equal(stored) {
				t.Errorf("RefreshExpires = %v, want the stored expiry %v", issued.RefreshExpires, stored)
			}
			ttl := testAuthService(nil).refreshTTL
			if stored.Before(before.Add(ttl)) || stored.After(after.Add(ttl)) {
				t.Errorf("expiry %v is not within refresh TTL %v of the call", stored, ttl)
			}
		})
	}
}
