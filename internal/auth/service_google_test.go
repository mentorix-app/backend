package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeIDTokenVerifier struct {
	claims IDTokenClaims
	err    error
}

func (f fakeIDTokenVerifier) Verify(context.Context, string) (IDTokenClaims, error) {
	return f.claims, f.err
}

func testGoogleService(store authStore, v IDTokenVerifier) *Service {
	svc := testAuthService(store)
	svc.googleVerifier = v
	return svc
}

func TestService_SignInWithGoogle_success(t *testing.T) {
	userID := uuid.New()
	store := &fakeAuthStore{
		clientUserID: userID,
		profile:      UserProfile{Email: "person@example.com"},
	}
	svc := testGoogleService(store, fakeIDTokenVerifier{claims: IDTokenClaims{
		Subject: "sub-1", Email: "  Person@Example.com ", EmailVerified: true, Name: "  Person Name ",
	}})

	issued, err := svc.SignInWithGoogle(context.Background(), "raw")
	if err != nil {
		t.Fatalf("SignInWithGoogle() error = %v", err)
	}
	if issued.UserID != userID {
		t.Errorf("user id = %v, want %v", issued.UserID, userID)
	}
	if issued.AccessToken == "" || issued.RefreshToken == "" {
		t.Fatal("expected access and refresh tokens")
	}
	if issued.Email != "person@example.com" {
		t.Errorf("email = %q", issued.Email)
	}
	want := ClientIdentity{Provider: ProviderGoogle, Subject: "sub-1", Email: "person@example.com", DisplayName: "Person Name"}
	if len(store.clientCalls) != 1 || store.clientCalls[0] != want {
		t.Errorf("store call = %+v, want [%+v]", store.clientCalls, want)
	}
}

func TestService_SignInWithGoogle_profileFields(t *testing.T) {
	tests := []struct {
		name      string
		claims    IDTokenClaims
		wantEmail string
		wantName  string
	}{
		{
			name:      "unverified email is not stored",
			claims:    IDTokenClaims{Subject: "s", Email: "a@b.com", EmailVerified: false, Name: "A"},
			wantEmail: "",
			wantName:  "A",
		},
		{
			name:      "missing name stays empty",
			claims:    IDTokenClaims{Subject: "s", Email: "a@b.com", EmailVerified: true},
			wantEmail: "a@b.com",
			wantName:  "",
		},
		{
			name:      "name is cut to 100 runes",
			claims:    IDTokenClaims{Subject: "s", Name: strings.Repeat("я", 150)},
			wantEmail: "",
			wantName:  strings.Repeat("я", 100),
		},
		{
			name:      "blank name after trim",
			claims:    IDTokenClaims{Subject: "s", Name: "   "},
			wantEmail: "",
			wantName:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{}
			svc := testGoogleService(store, fakeIDTokenVerifier{claims: tt.claims})
			if _, err := svc.SignInWithGoogle(context.Background(), "raw"); err != nil {
				t.Fatalf("SignInWithGoogle() error = %v", err)
			}
			got := store.clientCalls[0]
			if got.Email != tt.wantEmail || got.DisplayName != tt.wantName {
				t.Errorf("email, name = %q, %q; want %q, %q", got.Email, got.DisplayName, tt.wantEmail, tt.wantName)
			}
		})
	}
}

func TestService_SignInWithGoogle_notConfigured(t *testing.T) {
	store := &fakeAuthStore{}
	svc := testAuthService(store)
	_, err := svc.SignInWithGoogle(context.Background(), "raw")
	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("error = %v, want ErrProviderNotConfigured", err)
	}
	if len(store.clientCalls) != 0 {
		t.Error("store must not be called")
	}
}

func TestService_SignInWithGoogle_invalidToken(t *testing.T) {
	store := &fakeAuthStore{}
	svc := testGoogleService(store, fakeIDTokenVerifier{err: ErrInvalidIDToken})
	_, err := svc.SignInWithGoogle(context.Background(), "raw")
	if !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("error = %v, want ErrInvalidIDToken", err)
	}
	if len(store.clientCalls) != 0 {
		t.Error("store must not be called")
	}
}

func TestService_SignInWithGoogle_storeErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name  string
		store *fakeAuthStore
	}{
		{"find or create", &fakeAuthStore{clientErr: boom}},
		{"profile", &fakeAuthStore{profileErr: boom}},
		{"session", &fakeAuthStore{insertSession: boom}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := testGoogleService(tt.store, fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s"}})
			issued, err := svc.SignInWithGoogle(context.Background(), "raw")
			if !errors.Is(err, boom) {
				t.Fatalf("error = %v, want boom", err)
			}
			if issued.AccessToken != "" || issued.RefreshToken != "" {
				t.Error("no tokens on error")
			}
		})
	}
}

func testAppleService(store authStore, v IDTokenVerifier) *Service {
	svc := testAuthService(store)
	svc.appleVerifier = v
	return svc
}

func TestService_SignInWithApple_usesRequestName(t *testing.T) {
	tests := []struct {
		name      string
		tokenName string
		reqName   string
		want      string
	}{
		{"request name", "", "  Apple Person ", "Apple Person"},
		{"token name wins", "Token Name", "Request Name", "Token Name"},
		{"no name", "", "", ""},
		{"NUL removed from request name", "", "An\x00na", "Anna"},
		{"NUL removed from token name", "Jo\x00hn", "Request", "John"},
		{"only NUL and spaces falls back to empty", "", " \x00 \x00", ""},
		{"request name cut to 100 runes", "", strings.Repeat("я", 150), strings.Repeat("я", 100)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{profile: UserProfile{Email: "a@b.com"}}
			svc := testAppleService(store, fakeIDTokenVerifier{claims: IDTokenClaims{
				Subject: "apple-sub", Email: "A@B.com", EmailVerified: true, Name: tt.tokenName,
			}})
			issued, err := svc.SignInWithApple(context.Background(), "raw", tt.reqName)
			if err != nil {
				t.Fatalf("SignInWithApple() error = %v", err)
			}
			if issued.AccessToken == "" || issued.RefreshToken == "" {
				t.Fatal("expected access and refresh tokens")
			}
			want := ClientIdentity{Provider: ProviderApple, Subject: "apple-sub", Email: "a@b.com", DisplayName: tt.want}
			if len(store.clientCalls) != 1 || store.clientCalls[0] != want {
				t.Errorf("store call = %+v, want [%+v]", store.clientCalls, want)
			}
		})
	}
}

func TestService_SignInWithApple_notConfigured(t *testing.T) {
	store := &fakeAuthStore{}
	// A Google verifier alone must not enable Apple.
	svc := testGoogleService(store, fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s"}})
	_, err := svc.SignInWithApple(context.Background(), "raw", "")
	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("error = %v, want ErrProviderNotConfigured", err)
	}
	if len(store.clientCalls) != 0 {
		t.Error("store must not be called")
	}
}

func TestService_SignInWithApple_invalidToken(t *testing.T) {
	store := &fakeAuthStore{}
	svc := testAppleService(store, fakeIDTokenVerifier{err: ErrInvalidIDToken})
	if _, err := svc.SignInWithApple(context.Background(), "raw", "Name"); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("error = %v, want ErrInvalidIDToken", err)
	}
	if len(store.clientCalls) != 0 {
		t.Error("store must not be called")
	}
}
