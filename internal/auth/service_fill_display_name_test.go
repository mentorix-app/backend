package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestService_SignIn_fillsEmptyNameOfExistingAccount(t *testing.T) {
	tests := []struct {
		name     string
		signIn   func(svc *Service) (IssuedAuth, error)
		verifier IDTokenVerifier
		want     string
	}{
		{
			name: "google token name",
			signIn: func(svc *Service) (IssuedAuth, error) {
				return svc.SignInWithGoogle(context.Background(), "raw")
			},
			verifier: fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s", Name: "  Google Person "}},
			want:     "Google Person",
		},
		{
			name: "apple request name",
			signIn: func(svc *Service) (IssuedAuth, error) {
				return svc.SignInWithApple(context.Background(), "raw", "  Apple Person ")
			},
			verifier: fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s"}},
			want:     "Apple Person",
		},
		{
			name: "apple token name wins over request name",
			signIn: func(svc *Service) (IssuedAuth, error) {
				return svc.SignInWithApple(context.Background(), "raw", "Request Name")
			},
			verifier: fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s", Name: "Token Name"}},
			want:     "Token Name",
		},
		{
			name: "name is cleaned and cut to 100 runes",
			signIn: func(svc *Service) (IssuedAuth, error) {
				return svc.SignInWithApple(context.Background(), "raw", "\x00"+strings.Repeat("я", 150))
			},
			verifier: fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s"}},
			want:     strings.Repeat("я", 100),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			store := &fakeAuthStore{clientUserID: userID, profile: UserProfile{Name: ""}}
			svc := testAuthService(store)
			svc.googleVerifier = tt.verifier
			svc.appleVerifier = tt.verifier

			issued, err := tt.signIn(svc)
			if err != nil {
				t.Fatalf("sign-in error = %v", err)
			}
			if issued.UserID != userID {
				t.Errorf("user id = %v, want %v", issued.UserID, userID)
			}
			want := []fillDisplayNameCall{{userID, tt.want}}
			if len(store.fillCalls) != 1 || store.fillCalls[0] != want[0] {
				t.Errorf("fill calls = %+v, want %+v", store.fillCalls, want)
			}
			if store.profile.Name != tt.want {
				t.Errorf("stored name = %q, want %q", store.profile.Name, tt.want)
			}
		})
	}
}

func TestService_SignIn_keepsNameOfExistingAccount(t *testing.T) {
	tests := []struct {
		name   string
		signIn func(svc *Service) (IssuedAuth, error)
	}{
		{"google", func(svc *Service) (IssuedAuth, error) {
			return svc.SignInWithGoogle(context.Background(), "raw")
		}},
		{"apple", func(svc *Service) (IssuedAuth, error) {
			return svc.SignInWithApple(context.Background(), "raw", "Request Name")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{profile: UserProfile{Name: "Chosen Name"}}
			v := fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s", Name: "Token Name"}}
			svc := testAuthService(store)
			svc.googleVerifier = v
			svc.appleVerifier = v

			if _, err := tt.signIn(svc); err != nil {
				t.Fatalf("sign-in error = %v", err)
			}
			if len(store.fillCalls) != 1 {
				t.Errorf("fill calls = %d, want 1: the service asks and the store guard keeps the name", len(store.fillCalls))
			}
			if store.profile.Name != "Chosen Name" {
				t.Errorf("name = %q, want it unchanged", store.profile.Name)
			}
		})
	}
}

func TestService_SignIn_newAccountDoesNotFill(t *testing.T) {
	store := &fakeAuthStore{clientCreated: true}
	v := fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s", Name: "New Person"}}
	svc := testAuthService(store)
	svc.googleVerifier = v
	svc.appleVerifier = v

	if _, err := svc.SignInWithGoogle(context.Background(), "raw"); err != nil {
		t.Fatalf("SignInWithGoogle() error = %v", err)
	}
	if _, err := svc.SignInWithApple(context.Background(), "raw", "Request Name"); err != nil {
		t.Fatalf("SignInWithApple() error = %v", err)
	}
	if len(store.fillCalls) != 0 {
		t.Errorf("fill calls = %+v, want none for a new account", store.fillCalls)
	}
	if len(store.clientCalls) != 2 || store.clientCalls[0].DisplayName != "New Person" {
		t.Errorf("store calls = %+v, want the name passed on creation", store.clientCalls)
	}
}

func TestService_SignIn_emptyNameDoesNotFill(t *testing.T) {
	tests := []struct {
		name   string
		claims IDTokenClaims
		req    string
	}{
		{"no name anywhere", IDTokenClaims{Subject: "s"}, ""},
		{"blank names", IDTokenClaims{Subject: "s", Name: "   "}, "  "},
		{"only NUL and spaces", IDTokenClaims{Subject: "s"}, " \x00 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{}
			svc := testAuthService(store)
			svc.appleVerifier = fakeIDTokenVerifier{claims: tt.claims}

			if _, err := svc.SignInWithApple(context.Background(), "raw", tt.req); err != nil {
				t.Fatalf("SignInWithApple() error = %v", err)
			}
			if len(store.fillCalls) != 0 {
				t.Errorf("fill calls = %+v, want none", store.fillCalls)
			}
		})
	}
}

func TestService_SignIn_fillErrorFailsSignIn(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		signIn func(svc *Service) (IssuedAuth, error)
	}{
		{"google", func(svc *Service) (IssuedAuth, error) {
			return svc.SignInWithGoogle(context.Background(), "raw")
		}},
		{"apple", func(svc *Service) (IssuedAuth, error) {
			return svc.SignInWithApple(context.Background(), "raw", "")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{fillErr: boom}
			v := fakeIDTokenVerifier{claims: IDTokenClaims{Subject: "s", Name: "Token Name"}}
			svc := testAuthService(store)
			svc.googleVerifier = v
			svc.appleVerifier = v

			issued, err := tt.signIn(svc)
			if !errors.Is(err, boom) {
				t.Fatalf("error = %v, want boom", err)
			}
			if issued.AccessToken != "" || issued.RefreshToken != "" {
				t.Error("no tokens on error")
			}
		})
	}
}
