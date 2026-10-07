package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testOIDCIssuer   = "https://issuer.test"
	testOIDCClientID = "com.example.app"
	testOIDCKeyID    = "test-key-1"

	testKeyFetchTimeout = 5 * time.Second
)

// oidcFixture serves the public half of a locally generated RSA key as a JWKS
// document and signs ID tokens with the private half.
type oidcFixture struct {
	key      *rsa.PrivateKey
	server   *httptest.Server
	requests atomic.Int32
}

func newOIDCFixture(t *testing.T) *oidcFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	f := &oidcFixture{key: key}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.requests.Add(1)
		doc := map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": testOIDCKeyID,
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *oidcFixture) verifier(clientIDs ...string) IDTokenVerifier {
	return newOIDCVerifier(context.Background(), testOIDCIssuer, f.server.URL, clientIDs, testKeyFetchTimeout)
}

func (f *oidcFixture) sign(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testOIDCKeyID
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func validOIDCClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":            testOIDCIssuer,
		"sub":            "subject-123",
		"aud":            testOIDCClientID,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Add(-time.Minute).Unix(),
		"email":          "person@example.com",
		"email_verified": true,
		"name":           "Person Name",
	}
}

func TestOIDCVerifier_Verify(t *testing.T) {
	f := newOIDCFixture(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}

	tests := []struct {
		name    string
		token   func() string
		want    IDTokenClaims
		wantErr bool
	}{
		{
			name:  "valid token",
			token: func() string { return f.sign(t, f.key, validOIDCClaims()) },
			want:  IDTokenClaims{Subject: "subject-123", Email: "person@example.com", EmailVerified: true, Name: "Person Name"},
		},
		{
			name: "audience list with one allowed value",
			token: func() string {
				c := validOIDCClaims()
				c["aud"] = []string{"other.app", testOIDCClientID}
				return f.sign(t, f.key, c)
			},
			want: IDTokenClaims{Subject: "subject-123", Email: "person@example.com", EmailVerified: true, Name: "Person Name"},
		},
		{
			name: "email_verified as string true",
			token: func() string {
				c := validOIDCClaims()
				c["email_verified"] = "true"
				return f.sign(t, f.key, c)
			},
			want: IDTokenClaims{Subject: "subject-123", Email: "person@example.com", EmailVerified: true, Name: "Person Name"},
		},
		{
			name: "email_verified as string false",
			token: func() string {
				c := validOIDCClaims()
				c["email_verified"] = "false"
				return f.sign(t, f.key, c)
			},
			want: IDTokenClaims{Subject: "subject-123", Email: "person@example.com", EmailVerified: false, Name: "Person Name"},
		},
		{
			name: "email_verified as bool false",
			token: func() string {
				c := validOIDCClaims()
				c["email_verified"] = false
				return f.sign(t, f.key, c)
			},
			want: IDTokenClaims{Subject: "subject-123", Email: "person@example.com", EmailVerified: false, Name: "Person Name"},
		},
		{
			name: "no email claims",
			token: func() string {
				c := validOIDCClaims()
				delete(c, "email")
				delete(c, "email_verified")
				delete(c, "name")
				return f.sign(t, f.key, c)
			},
			want: IDTokenClaims{Subject: "subject-123"},
		},
		{
			name: "wrong audience",
			token: func() string {
				c := validOIDCClaims()
				c["aud"] = "someone.else"
				return f.sign(t, f.key, c)
			},
			wantErr: true,
		},
		{
			name: "wrong issuer",
			token: func() string {
				c := validOIDCClaims()
				c["iss"] = "https://evil.test"
				return f.sign(t, f.key, c)
			},
			wantErr: true,
		},
		{
			name: "expired",
			token: func() string {
				c := validOIDCClaims()
				c["exp"] = time.Now().Add(-time.Hour).Unix()
				return f.sign(t, f.key, c)
			},
			wantErr: true,
		},
		{
			name:    "bad signature",
			token:   func() string { return f.sign(t, otherKey, validOIDCClaims()) },
			wantErr: true,
		},
		{
			name: "empty subject",
			token: func() string {
				c := validOIDCClaims()
				c["sub"] = ""
				return f.sign(t, f.key, c)
			},
			wantErr: true,
		},
		{
			name:    "garbage",
			token:   func() string { return "not-a-jwt" },
			wantErr: true,
		},
		{
			name:    "empty",
			token:   func() string { return "" },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := f.verifier(testOIDCClientID)
			got, err := v.Verify(context.Background(), tt.token())
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidIDToken) {
					t.Fatalf("Verify() error = %v, want ErrInvalidIDToken", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("claims = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOIDCVerifier_Verify_audienceMustBeConfigured(t *testing.T) {
	f := newOIDCFixture(t)
	token := f.sign(t, f.key, validOIDCClaims())

	tests := []struct {
		name      string
		clientIDs []string
	}{
		{name: "empty list accepts nothing", clientIDs: nil},
		{name: "list without the audience", clientIDs: []string{"a.app", "b.app"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.verifier(tt.clientIDs...).Verify(context.Background(), token)
			if !errors.Is(err, ErrInvalidIDToken) {
				t.Fatalf("Verify() error = %v, want ErrInvalidIDToken", err)
			}
		})
	}

	if _, err := f.verifier("a.app", testOIDCClientID).Verify(context.Background(), token); err != nil {
		t.Fatalf("Verify() with audience in the list error = %v", err)
	}
}

func TestOIDCVerifier_fetchesKeysLazily(t *testing.T) {
	f := newOIDCFixture(t)
	v := f.verifier(testOIDCClientID)
	if got := f.requests.Load(); got != 0 {
		t.Fatalf("key requests after construction = %d, want 0", got)
	}
	if _, err := v.Verify(context.Background(), f.sign(t, f.key, validOIDCClaims())); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got := f.requests.Load(); got != 1 {
		t.Fatalf("key requests after first verify = %d, want 1", got)
	}
}

func TestOIDCVerifier_unreachableKeyServer(t *testing.T) {
	f := newOIDCFixture(t)
	token := f.sign(t, f.key, validOIDCClaims())
	f.server.Close()
	_, err := f.verifier(testOIDCClientID).Verify(context.Background(), token)
	if !errors.Is(err, ErrIDTokenProviderUnavailable) {
		t.Fatalf("Verify() error = %v, want ErrIDTokenProviderUnavailable", err)
	}
	if errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("Verify() error = %v must not also be ErrInvalidIDToken", err)
	}
}

func TestOIDCVerifier_keyServerErrorStatusIsUnavailable(t *testing.T) {
	f := newOIDCFixture(t)
	token := f.sign(t, f.key, validOIDCClaims())
	for _, status := range []int{http.StatusInternalServerError, http.StatusNotFound, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", status)
		}))
		v := newOIDCVerifier(context.Background(), testOIDCIssuer, srv.URL, []string{testOIDCClientID}, testKeyFetchTimeout)
		_, err := v.Verify(context.Background(), token)
		srv.Close()
		if !errors.Is(err, ErrIDTokenProviderUnavailable) || errors.Is(err, ErrInvalidIDToken) {
			t.Errorf("status %d: Verify() error = %v, want only ErrIDTokenProviderUnavailable", status, err)
		}
	}
}

func TestOIDCVerifier_slowKeyServerTimesOut(t *testing.T) {
	f := newOIDCFixture(t)
	token := f.sign(t, f.key, validOIDCClaims())
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slow.Close()
	const timeout = 100 * time.Millisecond
	v := newOIDCVerifier(context.Background(), testOIDCIssuer, slow.URL, []string{testOIDCClientID}, timeout)

	start := time.Now()
	_, err := v.Verify(context.Background(), token)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrIDTokenProviderUnavailable) {
		t.Fatalf("Verify() error = %v, want ErrIDTokenProviderUnavailable", err)
	}
	if elapsed > timeout+2*time.Second {
		t.Fatalf("Verify() took %v with a %v key fetch timeout", elapsed, timeout)
	}
}

func TestOIDCVerifier_badSignatureIsNotUnavailable(t *testing.T) {
	f := newOIDCFixture(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_, err = f.verifier(testOIDCClientID).Verify(context.Background(), f.sign(t, otherKey, validOIDCClaims()))
	if !errors.Is(err, ErrInvalidIDToken) || errors.Is(err, ErrIDTokenProviderUnavailable) {
		t.Fatalf("Verify() error = %v, want only ErrInvalidIDToken", err)
	}
}

func TestOIDCVerifier_rejectsForgedAlgorithms(t *testing.T) {
	f := newOIDCFixture(t)
	der, err := x509.MarshalPKIXPublicKey(&f.key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	signWith := func(method jwt.SigningMethod, key any) string {
		tok := jwt.NewWithClaims(method, validOIDCClaims())
		tok.Header["kid"] = testOIDCKeyID
		raw, err := tok.SignedString(key)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return raw
	}
	tests := map[string]string{
		"alg none":                signWith(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType),
		"HS256 with DER key":      signWith(jwt.SigningMethodHS256, der),
		"HS256 with PEM key":      signWith(jwt.SigningMethodHS256, pemBytes),
		"HS256 with modulus":      signWith(jwt.SigningMethodHS256, f.key.N.Bytes()),
		"RS256 header, none sign": strings.Join(strings.Split(f.sign(t, f.key, validOIDCClaims()), ".")[:2], ".") + ".",
	}
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := f.verifier(testOIDCClientID).Verify(context.Background(), token)
			if !errors.Is(err, ErrInvalidIDToken) {
				t.Fatalf("Verify() error = %v, want ErrInvalidIDToken", err)
			}
		})
	}
}

func TestOIDCVerifier_emailVerifiedAsNumberIsNeverVerified(t *testing.T) {
	f := newOIDCFixture(t)
	for _, v := range []any{1, 0, 1.5} {
		c := validOIDCClaims()
		c["email_verified"] = v
		got, err := f.verifier(testOIDCClientID).Verify(context.Background(), f.sign(t, f.key, c))
		if err == nil && got.EmailVerified {
			t.Errorf("email_verified %v treated as verified", v)
		}
		if err != nil && !errors.Is(err, ErrInvalidIDToken) {
			t.Errorf("email_verified %v: error = %v, want ErrInvalidIDToken", v, err)
		}
	}
}

func TestOIDCVerifier_googleAcceptsIssuerWithoutScheme(t *testing.T) {
	f := newOIDCFixture(t)
	v := newOIDCVerifier(context.Background(), "https://accounts.google.com", f.server.URL, []string{testOIDCClientID}, testKeyFetchTimeout)
	c := validOIDCClaims()
	c["iss"] = "accounts.google.com"
	if _, err := v.Verify(context.Background(), f.sign(t, f.key, c)); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestNewSocialVerifiers(t *testing.T) {
	tests := []struct {
		name   string
		apple  []string
		google []string
		want   []string
	}{
		{name: "none configured", want: nil},
		{name: "google only", google: []string{"g"}, want: []string{ProviderGoogle}},
		{name: "apple only", apple: []string{"a"}, want: []string{ProviderApple}},
		{name: "both", apple: []string{"a"}, google: []string{"g"}, want: []string{ProviderApple, ProviderGoogle}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSocialVerifiers(context.Background(), tt.apple, tt.google)
			if len(got) != len(tt.want) {
				t.Fatalf("providers = %v, want %v", got, tt.want)
			}
			for _, p := range tt.want {
				if got[p] == nil {
					t.Errorf("verifier for %q is nil", p)
				}
			}
		})
	}
}

// brokenKeyServer answers 200 with a JWKS body that fails in the way given.
func brokenKeyServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestOIDCVerifier_brokenKeyResponseIsUnavailable(t *testing.T) {
	f := newOIDCFixture(t)
	token := f.sign(t, f.key, validOIDCClaims())
	tests := map[string]http.HandlerFunc{
		"truncated body": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"keys":[`))
		},
		"not json": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		},
		"empty body": func(http.ResponseWriter, *http.Request) {},
		"body over the limit": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"keys":[],"pad":"`))
			_, _ = w.Write([]byte(strings.Repeat("a", maxKeySetBytes)))
			_, _ = w.Write([]byte(`"}`))
		},
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			srv := brokenKeyServer(t, h)
			v := newOIDCVerifier(context.Background(), testOIDCIssuer, srv.URL, []string{testOIDCClientID}, testKeyFetchTimeout)
			_, err := v.Verify(context.Background(), token)
			if !errors.Is(err, ErrIDTokenProviderUnavailable) || errors.Is(err, ErrInvalidIDToken) {
				t.Fatalf("Verify() error = %v, want only ErrIDTokenProviderUnavailable", err)
			}
		})
	}
}

func TestOIDCVerifier_stalledKeyBodyTimesOut(t *testing.T) {
	f := newOIDCFixture(t)
	token := f.sign(t, f.key, validOIDCClaims())
	srv := brokenKeyServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"keys":[`))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	const timeout = 200 * time.Millisecond
	v := newOIDCVerifier(context.Background(), testOIDCIssuer, srv.URL, []string{testOIDCClientID}, timeout)

	start := time.Now()
	_, err := v.Verify(context.Background(), token)

	if !errors.Is(err, ErrIDTokenProviderUnavailable) {
		t.Fatalf("Verify() error = %v, want ErrIDTokenProviderUnavailable", err)
	}
	if elapsed := time.Since(start); elapsed > timeout+2*time.Second {
		t.Fatalf("Verify() took %v", elapsed)
	}
}

func TestOIDCVerifier_cachedKeySurvivesKeyServerOutage(t *testing.T) {
	f := newOIDCFixture(t)
	v := f.verifier(testOIDCClientID)
	if _, err := v.Verify(context.Background(), f.sign(t, f.key, validOIDCClaims())); err != nil {
		t.Fatalf("first Verify() error = %v", err)
	}
	f.server.Close()
	if _, err := v.Verify(context.Background(), f.sign(t, f.key, validOIDCClaims())); err != nil {
		t.Fatalf("Verify() with a cached key after the outage error = %v", err)
	}
}

func TestOIDCVerifier_concurrentValidAndBadTokens(t *testing.T) {
	f := newOIDCFixture(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	good := f.sign(t, f.key, validOIDCClaims())
	bad := f.sign(t, otherKey, validOIDCClaims())
	v := f.verifier(testOIDCClientID)

	const n = 40
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token := good
			if i%2 == 1 {
				token = bad
			}
			_, errs[i] = v.Verify(context.Background(), token)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if i%2 == 0 && err != nil {
			t.Errorf("valid token %d: error = %v", i, err)
		}
		if i%2 == 1 && (!errors.Is(err, ErrInvalidIDToken) || errors.Is(err, ErrIDTokenProviderUnavailable)) {
			t.Errorf("bad token %d: error = %v, want only ErrInvalidIDToken", i, err)
		}
	}
}

func TestOIDCVerifier_concurrentOutageDoesNotLeakIntoHealthyVerifier(t *testing.T) {
	f := newOIDCFixture(t)
	token := f.sign(t, f.key, validOIDCClaims())
	down := brokenKeyServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	})
	broken := newOIDCVerifier(context.Background(), testOIDCIssuer, down.URL, []string{testOIDCClientID}, testKeyFetchTimeout)
	healthy := f.verifier(testOIDCClientID)

	const n = 40
	brokenErrs := make([]error, n)
	healthyErrs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, brokenErrs[i] = broken.Verify(context.Background(), token)
		}()
		go func() {
			defer wg.Done()
			_, healthyErrs[i] = healthy.Verify(context.Background(), token)
		}()
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if !errors.Is(brokenErrs[i], ErrIDTokenProviderUnavailable) || errors.Is(brokenErrs[i], ErrInvalidIDToken) {
			t.Errorf("down provider call %d: error = %v", i, brokenErrs[i])
		}
		if healthyErrs[i] != nil {
			t.Errorf("healthy provider call %d: error = %v", i, healthyErrs[i])
		}
	}
}

func TestOIDCVerifier_outageDoesNotTaintLaterCallsOnSameVerifier(t *testing.T) {
	f := newOIDCFixture(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var down atomic.Bool
	down.Store(true)
	srv := brokenKeyServer(t, func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		f.server.Config.Handler.ServeHTTP(w, r)
	})
	v := newOIDCVerifier(context.Background(), testOIDCIssuer, srv.URL, []string{testOIDCClientID}, testKeyFetchTimeout)

	if _, err := v.Verify(context.Background(), f.sign(t, f.key, validOIDCClaims())); !errors.Is(err, ErrIDTokenProviderUnavailable) {
		t.Fatalf("during outage: error = %v, want ErrIDTokenProviderUnavailable", err)
	}
	down.Store(false)

	_, err = v.Verify(context.Background(), f.sign(t, otherKey, validOIDCClaims()))
	if !errors.Is(err, ErrInvalidIDToken) || errors.Is(err, ErrIDTokenProviderUnavailable) {
		t.Fatalf("bad token after recovery: error = %v, want only ErrInvalidIDToken", err)
	}
	if _, err := v.Verify(context.Background(), f.sign(t, f.key, validOIDCClaims())); err != nil {
		t.Fatalf("good token after recovery: error = %v", err)
	}
}
