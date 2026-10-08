package auth

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	googleIssuer  = "https://accounts.google.com"
	googleKeysURL = "https://www.googleapis.com/oauth2/v3/certs"

	keyFetchTimeout = 5 * time.Second
)

// IDTokenClaims are the verified claims the sign-in flow uses.
type IDTokenClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// IDTokenVerifier checks a provider ID token and returns its claims.
// Every failure wraps ErrInvalidIDToken.
type IDTokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (IDTokenClaims, error)
}

type oidcVerifier struct {
	verifier *oidc.IDTokenVerifier
	audience []string
}

// NewGoogleIDTokenVerifier accepts Google ID tokens issued for one of clientIDs.
func NewGoogleIDTokenVerifier(clientIDs []string) IDTokenVerifier {
	return newOIDCVerifier(googleIssuer, googleKeysURL, clientIDs)
}

func newOIDCVerifier(issuer, keysURL string, clientIDs []string) *oidcVerifier {
	// The key set keeps this context for its refetches, so it must outlive any request.
	ctx := oidc.ClientContext(context.Background(), &http.Client{Timeout: keyFetchTimeout})
	keySet := oidc.NewRemoteKeySet(ctx, keysURL)
	return &oidcVerifier{
		// The audience is matched against a list below, which go-oidc cannot do.
		verifier: oidc.NewVerifier(issuer, keySet, &oidc.Config{SkipClientIDCheck: true}),
		audience: clientIDs,
	}
}

func (v *oidcVerifier) Verify(ctx context.Context, rawToken string) (IDTokenClaims, error) {
	tok, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return IDTokenClaims{}, fmt.Errorf("%w: %v", ErrInvalidIDToken, err)
	}
	if !slices.ContainsFunc(tok.Audience, func(a string) bool { return slices.Contains(v.audience, a) }) {
		return IDTokenClaims{}, fmt.Errorf("%w: audience not allowed", ErrInvalidIDToken)
	}
	if tok.Subject == "" {
		return IDTokenClaims{}, fmt.Errorf("%w: empty subject", ErrInvalidIDToken)
	}
	var c struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := tok.Claims(&c); err != nil {
		return IDTokenClaims{}, fmt.Errorf("%w: read claims: %v", ErrInvalidIDToken, err)
	}
	return IDTokenClaims{Subject: tok.Subject, Email: c.Email, EmailVerified: c.EmailVerified, Name: c.Name}, nil
}
