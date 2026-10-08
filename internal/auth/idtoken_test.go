package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIDTokenIssuer   = "https://issuer.test"
	testIDTokenClientID = "app.client.id"
	testIDTokenKeyID    = "test-key-1"
)

// idTokenFixture serves the public half of a local RSA key as a JWKS document
// and signs ID tokens with the private half.
type idTokenFixture struct {
	key    *rsa.PrivateKey
	server *httptest.Server
}

func newIDTokenFixture(t *testing.T) *idTokenFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	f := &idTokenFixture{key: key}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		doc := map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": testIDTokenKeyID,
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *idTokenFixture) verifier(clientIDs ...string) IDTokenVerifier {
	return newOIDCVerifier(testIDTokenIssuer, f.server.URL, clientIDs)
}

func (f *idTokenFixture) sign(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testIDTokenKeyID
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func validIDTokenClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":            testIDTokenIssuer,
		"sub":            "subject-123",
		"aud":            testIDTokenClientID,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Add(-time.Minute).Unix(),
		"email":          "person@example.com",
		"email_verified": true,
		"name":           "Person Name",
	}
}

func TestIDTokenVerifier_Verify(t *testing.T) {
	f := newIDTokenFixture(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	want := IDTokenClaims{Subject: "subject-123", Email: "person@example.com", EmailVerified: true, Name: "Person Name"}

	tests := []struct {
		name    string
		token   func() string
		wantErr bool
	}{
		{
			name:  "valid token",
			token: func() string { return f.sign(t, f.key, validIDTokenClaims()) },
		},
		{
			name: "audience list with one allowed value",
			token: func() string {
				c := validIDTokenClaims()
				c["aud"] = []string{"other.app", testIDTokenClientID}
				return f.sign(t, f.key, c)
			},
		},
		{
			name: "wrong audience",
			token: func() string {
				c := validIDTokenClaims()
				c["aud"] = "someone.else"
				return f.sign(t, f.key, c)
			},
			wantErr: true,
		},
		{
			name: "wrong issuer",
			token: func() string {
				c := validIDTokenClaims()
				c["iss"] = "https://evil.test"
				return f.sign(t, f.key, c)
			},
			wantErr: true,
		},
		{
			name: "expired",
			token: func() string {
				c := validIDTokenClaims()
				c["exp"] = time.Now().Add(-time.Hour).Unix()
				return f.sign(t, f.key, c)
			},
			wantErr: true,
		},
		{
			name:    "bad signature",
			token:   func() string { return f.sign(t, otherKey, validIDTokenClaims()) },
			wantErr: true,
		},
		{
			name: "empty subject",
			token: func() string {
				c := validIDTokenClaims()
				c["sub"] = ""
				return f.sign(t, f.key, c)
			},
			wantErr: true,
		},
		{
			name:    "not a jwt",
			token:   func() string { return "garbage" },
			wantErr: true,
		},
		{
			name:    "empty token",
			token:   func() string { return "" },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := f.verifier(testIDTokenClientID).Verify(context.Background(), tt.token())
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidIDToken) {
					t.Fatalf("error = %v, want ErrInvalidIDToken", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if got != want {
				t.Errorf("claims = %+v, want %+v", got, want)
			}
		})
	}
}

func TestIDTokenVerifier_keyServerDown(t *testing.T) {
	f := newIDTokenFixture(t)
	v := f.verifier(testIDTokenClientID)
	raw := f.sign(t, f.key, validIDTokenClaims())
	f.server.Close()

	if _, err := v.Verify(context.Background(), raw); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("error = %v, want ErrInvalidIDToken", err)
	}
}

func TestIDTokenVerifier_noAllowedAudiences(t *testing.T) {
	f := newIDTokenFixture(t)
	raw := f.sign(t, f.key, validIDTokenClaims())

	if _, err := f.verifier().Verify(context.Background(), raw); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("error = %v, want ErrInvalidIDToken", err)
	}
}

func TestIDTokenVerifier_emailVerifiedForms(t *testing.T) {
	f := newIDTokenFixture(t)
	tests := []struct {
		name    string
		value   any
		want    bool
		wantErr bool
	}{
		{"boolean true", true, true, false},
		{"boolean false", false, false, false},
		{"string true", "true", true, false},
		{"string false", "false", false, false},
		{"null", nil, false, false},
		{"bad string", "yes", false, true},
		{"number", 1, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validIDTokenClaims()
			c["email_verified"] = tt.value
			got, err := f.verifier(testIDTokenClientID).Verify(context.Background(), f.sign(t, f.key, c))
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidIDToken) {
					t.Fatalf("error = %v, want ErrInvalidIDToken", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if got.EmailVerified != tt.want {
				t.Errorf("EmailVerified = %v, want %v", got.EmailVerified, tt.want)
			}
		})
	}
}

func TestIDTokenVerifier_appleIssuer(t *testing.T) {
	f := newIDTokenFixture(t)
	v := newOIDCVerifier(appleIssuer, f.server.URL, []string{testIDTokenClientID})
	c := validIDTokenClaims()
	c["iss"] = appleIssuer
	c["email_verified"] = "true"

	got, err := v.Verify(context.Background(), f.sign(t, f.key, c))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got.Subject != "subject-123" || !got.EmailVerified {
		t.Errorf("claims = %+v", got)
	}
	if _, err := v.Verify(context.Background(), f.sign(t, f.key, validIDTokenClaims())); !errors.Is(err, ErrInvalidIDToken) {
		t.Errorf("token with another issuer: error = %v, want ErrInvalidIDToken", err)
	}
	if NewAppleIDTokenVerifier([]string{"x"}) == nil {
		t.Error("NewAppleIDTokenVerifier returned nil")
	}
}
