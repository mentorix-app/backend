package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	googleIssuer  = "https://accounts.google.com"
	googleKeysURL = "https://www.googleapis.com/oauth2/v3/certs"
	appleIssuer   = "https://appleid.apple.com"
	appleKeysURL  = "https://appleid.apple.com/auth/keys"
)

// keyFetchTimeout bounds one request to a provider's key endpoint. go-oidc
// would otherwise use http.DefaultClient, which has no timeout.
const keyFetchTimeout = 5 * time.Second

// maxKeySetBytes caps a key endpoint response; real key sets are a few KiB.
const maxKeySetBytes = 1 << 20

var (
	// ErrInvalidIDToken wraps every ID token verification failure caused by the token.
	ErrInvalidIDToken = errors.New("invalid id token")
	// ErrIDTokenProviderUnavailable means the provider's signing keys could not
	// be fetched, so the token was not judged. It is never combined with ErrInvalidIDToken.
	ErrIDTokenProviderUnavailable = errors.New("id token provider unavailable")
)

// IDTokenClaims holds the claims of a verified ID token that sign-in uses.
type IDTokenClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// IDTokenVerifier checks the signature, issuer, audience and expiry of an
// identity provider's ID token. Implementations are safe for concurrent use.
type IDTokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (IDTokenClaims, error)
}

type oidcVerifier struct {
	verifier  *oidc.IDTokenVerifier
	audiences []string
}

// fetchFailureKey carries a per-call *fetchFailure through go-oidc to the key set.
type fetchFailureKey struct{}

// fetchFailure records, for one Verify call, that the key fetch failed.
// go-oidc flattens the key set error into text with %v, so the typed error
// cannot be recovered from Verify's result and travels through the context.
type fetchFailure struct{ err error }

// statusCheckTransport makes every failed key request reach the key set as a
// *url.Error. A non-200 answer, and a 200 whose body cannot be read in full,
// exceeds maxKeySetBytes or is not JSON, fail inside RoundTrip. go-oidc gets
// a buffered copy of a good body.
type statusCheckTransport struct{ base http.RoundTripper }

func (t statusCheckTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("key server answered %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxKeySetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read key response: %w", err)
	}
	if len(data) > maxKeySetBytes {
		return nil, fmt.Errorf("key response is larger than %d bytes", maxKeySetBytes)
	}
	if !json.Valid(data) {
		return nil, errors.New("key response is not valid JSON")
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	resp.ContentLength = int64(len(data))
	return resp, nil
}

// fetchAwareKeySet wraps the remote key set and notes key fetch failures.
// A failed fetch is a *url.Error (transport error, timeout or non-200 status) or
// the context error of the waiting caller. A bad signature or an unknown key
// is neither, and stays an invalid token.
type fetchAwareKeySet struct{ inner oidc.KeySet }

func (k fetchAwareKeySet) VerifySignature(ctx context.Context, jwt string) ([]byte, error) {
	payload, err := k.inner.VerifySignature(ctx, jwt)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if f, ok := ctx.Value(fetchFailureKey{}).(*fetchFailure); ok {
				f.err = err
			}
		}
	}
	return payload, err
}

// newOIDCVerifier builds a verifier for one provider. Signing keys are fetched
// from jwksURL on the first Verify call, not here, with each request limited to
// fetchTimeout. The audience check is done in Verify against clientIDs, so a
// token is accepted when at least one of its audiences is in the list.
func newOIDCVerifier(ctx context.Context, issuer, jwksURL string, clientIDs []string, fetchTimeout time.Duration) *oidcVerifier {
	client := &http.Client{
		Timeout:   fetchTimeout,
		Transport: statusCheckTransport{base: http.DefaultTransport},
	}
	keySet := fetchAwareKeySet{inner: oidc.NewRemoteKeySet(oidc.ClientContext(ctx, client), jwksURL)}
	return &oidcVerifier{
		verifier:  oidc.NewVerifier(issuer, keySet, &oidc.Config{SkipClientIDCheck: true}),
		audiences: clientIDs,
	}
}

// flexBool decodes a JSON boolean or the strings "true" and "false". Apple
// sends email_verified in either form.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	var v bool
	if err := json.Unmarshal(data, &v); err == nil {
		*b = flexBool(v)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("email_verified is neither bool nor string: %w", err)
	}
	*b = s == "true"
	return nil
}

func (v *oidcVerifier) Verify(ctx context.Context, rawToken string) (IDTokenClaims, error) {
	failure := &fetchFailure{}
	tok, err := v.verifier.Verify(context.WithValue(ctx, fetchFailureKey{}, failure), rawToken)
	if err != nil {
		if failure.err != nil {
			return IDTokenClaims{}, fmt.Errorf("fetch signing keys: %w: %w", ErrIDTokenProviderUnavailable, failure.err)
		}
		return IDTokenClaims{}, fmt.Errorf("verify id token: %w: %w", ErrInvalidIDToken, err)
	}
	if !slices.ContainsFunc(tok.Audience, func(a string) bool { return slices.Contains(v.audiences, a) }) {
		return IDTokenClaims{}, fmt.Errorf("audience not allowed: %w", ErrInvalidIDToken)
	}
	if tok.Subject == "" {
		return IDTokenClaims{}, fmt.Errorf("missing subject: %w", ErrInvalidIDToken)
	}
	var c struct {
		Email         string   `json:"email"`
		EmailVerified flexBool `json:"email_verified"`
		Name          string   `json:"name"`
	}
	if err := tok.Claims(&c); err != nil {
		return IDTokenClaims{}, fmt.Errorf("decode claims: %w: %w", ErrInvalidIDToken, err)
	}
	return IDTokenClaims{
		Subject:       tok.Subject,
		Email:         c.Email,
		EmailVerified: bool(c.EmailVerified),
		Name:          c.Name,
	}, nil
}

// NewSocialVerifiers builds a verifier for each provider that has at least one
// allowed client ID, keyed by provider name. A provider with an empty list is
// left out and sign-in with it reports ErrProviderNotConfigured.
func NewSocialVerifiers(ctx context.Context, appleClientIDs, googleClientIDs []string) map[string]IDTokenVerifier {
	out := make(map[string]IDTokenVerifier, 2)
	if len(appleClientIDs) > 0 {
		out[ProviderApple] = newOIDCVerifier(ctx, appleIssuer, appleKeysURL, appleClientIDs, keyFetchTimeout)
	}
	if len(googleClientIDs) > 0 {
		out[ProviderGoogle] = newOIDCVerifier(ctx, googleIssuer, googleKeysURL, googleClientIDs, keyFetchTimeout)
	}
	return out
}
