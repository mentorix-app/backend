package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func signInFuncs() map[string]func(svc *Service) (IssuedAuth, error) {
	return map[string]func(svc *Service) (IssuedAuth, error){
		"google": func(svc *Service) (IssuedAuth, error) {
			return svc.SignInWithGoogle(context.Background(), "raw", "")
		},
		"apple": func(svc *Service) (IssuedAuth, error) {
			return svc.SignInWithApple(context.Background(), "raw", "", testAppleNonce)
		},
	}
}

func TestService_SignIn_fillsEmptyEmailOfExistingAccount(t *testing.T) {
	for provider, signIn := range signInFuncs() {
		t.Run(provider, func(t *testing.T) {
			userID := uuid.New()
			store := &fakeAuthStore{clientUserID: userID}
			v := fakeIDTokenVerifier{claims: IDTokenClaims{
				Subject: "s", Email: "  User@Example.COM ", EmailVerified: true, Nonce: hashedNonce(testAppleNonce),
			}}
			svc := testAuthService(store)
			svc.googleVerifier = v
			svc.appleVerifier = v

			issued, err := signIn(svc)
			if err != nil {
				t.Fatalf("sign-in error = %v", err)
			}
			want := fillEmailCall{userID, "user@example.com"}
			if len(store.emailFillCalls) != 1 || store.emailFillCalls[0] != want {
				t.Errorf("email fill calls = %+v, want [%+v]", store.emailFillCalls, want)
			}
			if issued.Email != want.email {
				t.Errorf("issued email = %q, want %q", issued.Email, want.email)
			}
		})
	}
}

func TestService_SignIn_keepsEmailOfExistingAccount(t *testing.T) {
	for provider, signIn := range signInFuncs() {
		t.Run(provider, func(t *testing.T) {
			store := &fakeAuthStore{profile: UserProfile{Email: "old@example.com"}}
			v := fakeIDTokenVerifier{claims: IDTokenClaims{
				Subject: "s", Email: "new@example.com", EmailVerified: true, Nonce: hashedNonce(testAppleNonce),
			}}
			svc := testAuthService(store)
			svc.googleVerifier = v
			svc.appleVerifier = v

			issued, err := signIn(svc)
			if err != nil {
				t.Fatalf("sign-in error = %v", err)
			}
			if len(store.emailFillCalls) != 1 {
				t.Errorf("email fill calls = %d, want 1: the service asks and the store guard keeps the email", len(store.emailFillCalls))
			}
			if store.profile.Email != "old@example.com" || issued.Email != "old@example.com" {
				t.Errorf("stored email = %q, issued email = %q, want old@example.com for both", store.profile.Email, issued.Email)
			}
		})
	}
}

func TestService_SignIn_emailThatIsNotUsableDoesNotFill(t *testing.T) {
	tests := []struct {
		name   string
		claims IDTokenClaims
	}{
		{"unverified email", IDTokenClaims{Subject: "s", Email: "user@example.com", EmailVerified: false, Nonce: hashedNonce(testAppleNonce)}},
		{"no email", IDTokenClaims{Subject: "s", EmailVerified: true, Nonce: hashedNonce(testAppleNonce)}},
		{"blank email", IDTokenClaims{Subject: "s", Email: "   ", EmailVerified: true, Nonce: hashedNonce(testAppleNonce)}},
	}
	for _, tt := range tests {
		for provider, signIn := range signInFuncs() {
			t.Run(tt.name+" "+provider, func(t *testing.T) {
				store := &fakeAuthStore{}
				svc := testAuthService(store)
				svc.googleVerifier = fakeIDTokenVerifier{claims: tt.claims}
				svc.appleVerifier = fakeIDTokenVerifier{claims: tt.claims}

				issued, err := signIn(svc)
				if err != nil {
					t.Fatalf("sign-in error = %v", err)
				}
				if len(store.emailFillCalls) != 0 {
					t.Errorf("email fill calls = %+v, want none", store.emailFillCalls)
				}
				if issued.Email != "" {
					t.Errorf("issued email = %q, want empty", issued.Email)
				}
			})
		}
	}
}

func TestService_SignIn_newAccountDoesNotFillEmail(t *testing.T) {
	store := &fakeAuthStore{clientCreated: true}
	v := fakeIDTokenVerifier{claims: IDTokenClaims{
		Subject: "s", Email: "new@example.com", EmailVerified: true, Nonce: hashedNonce(testAppleNonce),
	}}
	svc := testAuthService(store)
	svc.googleVerifier = v
	svc.appleVerifier = v

	for provider, signIn := range signInFuncs() {
		if _, err := signIn(svc); err != nil {
			t.Fatalf("%s sign-in error = %v", provider, err)
		}
	}
	if len(store.emailFillCalls) != 0 {
		t.Errorf("email fill calls = %+v, want none for a new account", store.emailFillCalls)
	}
	if len(store.clientCalls) != 2 {
		t.Fatalf("client calls = %d, want 2 (google and apple)", len(store.clientCalls))
	}
	for _, c := range store.clientCalls {
		if c.Email != "new@example.com" {
			t.Errorf("client call email = %q, want it passed on creation", c.Email)
		}
	}
}

func TestService_SignIn_emailFillErrorFailsSignIn(t *testing.T) {
	boom := errors.New("boom")
	for provider, signIn := range signInFuncs() {
		t.Run(provider, func(t *testing.T) {
			store := &fakeAuthStore{emailFillErr: boom}
			v := fakeIDTokenVerifier{claims: IDTokenClaims{
				Subject: "s", Email: "user@example.com", EmailVerified: true, Nonce: hashedNonce(testAppleNonce),
			}}
			svc := testAuthService(store)
			svc.googleVerifier = v
			svc.appleVerifier = v

			issued, err := signIn(svc)
			if !errors.Is(err, boom) {
				t.Fatalf("error = %v, want boom", err)
			}
			if issued.AccessToken != "" || issued.RefreshToken != "" {
				t.Error("no tokens on error")
			}
		})
	}
}
