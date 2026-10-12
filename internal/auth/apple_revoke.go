package auth

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	appleTokenURL  = "https://appleid.apple.com/auth/token"
	appleRevokeURL = "https://appleid.apple.com/auth/revoke"

	// appleRequestTimeout is the timeout of one HTTP call and the budget of a whole Revoke,
	// which makes two calls: the server's write timeout is 15 seconds.
	appleRequestTimeout  = 10 * time.Second
	appleClientSecretTTL = 5 * time.Minute
	// appleResponseMaxBytes bounds what is read from an answer. Apple's answers are a few
	// kilobytes at most.
	appleResponseMaxBytes = 64 << 10

	appleExchangeStep = "token exchange"
	appleRevokeStep   = "token revocation"
)

// AppleRevoker revokes the Sign in with Apple authorization behind an authorization code.
// Implementations are safe for concurrent use.
type AppleRevoker interface {
	// Revoke trades code for a refresh token and revokes it. clientID is the audience of
	// the ID token that came with the code. subject is the Apple account the code has to
	// belong to: the exchange answer carries an ID token of the account the code was
	// issued to, and nothing is revoked unless that is subject. redirectURI is empty
	// unless the authorization request had one. The whole call has one time budget.
	//
	// Failures wrap ErrInvalidAppleAuthorizationCode (Apple answered invalid_grant to the
	// code), ErrAppleAccountMismatch (the code belongs to another Apple account) or
	// ErrAppleRevocationFailed. No error text repeats the code, a token, a claim or the
	// client secret.
	Revoke(ctx context.Context, clientID, subject, code, redirectURI string) error
}

type appleRevokeClient struct {
	teamID, keyID       string
	key                 *ecdsa.PrivateKey
	client              *http.Client
	tokenURL, revokeURL string
	now                 func() time.Time
	// budget bounds one whole Revoke, both calls together.
	budget time.Duration
}

// NewAppleRevoker returns an AppleRevoker that signs its client secret with the team's
// key. Each request times out after 10 seconds, and one Revoke, with its two requests,
// has a budget of 10 seconds in all. Redirects are not followed: the forms hold the
// code and the tokens.
func NewAppleRevoker(teamID, keyID string, key *ecdsa.PrivateKey) AppleRevoker {
	return &appleRevokeClient{
		teamID: teamID,
		keyID:  keyID,
		key:    key,
		client: &http.Client{
			Timeout:       appleRequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		tokenURL:  appleTokenURL,
		revokeURL: appleRevokeURL,
		now:       time.Now,
		budget:    appleRequestTimeout,
	}
}

func (c *appleRevokeClient) Revoke(ctx context.Context, clientID, subject, code, redirectURI string) error {
	ctx, cancel := context.WithTimeout(ctx, c.budget)
	defer cancel()
	secret, err := c.clientSecret(clientID)
	if err != nil {
		return fmt.Errorf("%w: sign client secret: %w", ErrAppleRevocationFailed, err)
	}

	exchange := url.Values{
		"client_id":     {clientID},
		"client_secret": {secret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}
	if redirectURI != "" {
		exchange.Set("redirect_uri", redirectURI)
	}
	answer, err := c.post(ctx, appleExchangeStep, c.tokenURL, exchange)
	if err != nil {
		return err
	}
	if !answer.ok() {
		if answer.errorValue() == "invalid_grant" {
			return fmt.Errorf("%w: %s: %s", ErrInvalidAppleAuthorizationCode, appleExchangeStep, answer.describe())
		}
		return fmt.Errorf("%w: %s: %s", ErrAppleRevocationFailed, appleExchangeStep, answer.describe())
	}
	var tokens struct {
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	// The decoding error is dropped on purpose: its text can quote a piece of the body.
	if json.Unmarshal(answer.body, &tokens) != nil || tokens.RefreshToken == "" {
		return fmt.Errorf("%w: %s: the answer has no refresh token", ErrAppleRevocationFailed, appleExchangeStep)
	}

	if err := checkExchangeIDToken(tokens.IDToken, clientID, subject); err != nil {
		return err
	}

	answer, err = c.post(ctx, appleRevokeStep, c.revokeURL, url.Values{
		"client_id":       {clientID},
		"client_secret":   {secret},
		"token":           {tokens.RefreshToken},
		"token_type_hint": {"refresh_token"},
	})
	if err != nil {
		return err
	}
	if !answer.ok() {
		return fmt.Errorf("%w: %s: %s", ErrAppleRevocationFailed, appleRevokeStep, answer.describe())
	}
	return nil
}

// clientSecret is the JWT Apple takes as the client secret. The claims are a map so
// that aud is the plain string of Apple's example, not a one-element array.
func (c *appleRevokeClient) clientSecret(clientID string) (string, error) {
	now := c.now()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": c.teamID,
		"iat": now.Unix(),
		"exp": now.Add(appleClientSecretTTL).Unix(),
		"aud": appleIssuer,
		"sub": clientID,
	})
	token.Header["kid"] = c.keyID
	return token.SignedString(c.key)
}

// exchangeIDTokenClaims are the claims of the ID token in an exchange answer that matter
// here. Audience reads both forms of aud, a string and an array.
type exchangeIDTokenClaims struct {
	Issuer   string           `json:"iss"`
	Subject  string           `json:"sub"`
	Audience jwt.ClaimStrings `json:"aud"`
}

// checkExchangeIDToken makes sure the code of the exchange was issued to subject for
// clientID, by reading the ID token Apple returned with the tokens. The signature is not
// checked: the token came from Apple's host over TLS in answer to our own request, and
// its claims are only compared, never trusted to sign anyone in. A token that cannot be
// read, or that is for another issuer or client, is a failed revocation; a token of
// another subject is ErrAppleAccountMismatch. No error text repeats the token or a claim.
func checkExchangeIDToken(raw, clientID, subject string) error {
	claims, ok := readExchangeIDToken(raw)
	switch {
	case !ok:
		return fmt.Errorf("%w: %s: the answer has no readable id_token", ErrAppleRevocationFailed, appleExchangeStep)
	case claims.Issuer != appleIssuer:
		return fmt.Errorf("%w: %s: the id_token of the answer has another issuer", ErrAppleRevocationFailed, appleExchangeStep)
	case !slices.Contains(claims.Audience, clientID):
		return fmt.Errorf("%w: %s: the id_token of the answer is for another client", ErrAppleRevocationFailed, appleExchangeStep)
	case claims.Subject == "":
		return fmt.Errorf("%w: %s: the id_token of the answer has no subject", ErrAppleRevocationFailed, appleExchangeStep)
	case claims.Subject != subject:
		return fmt.Errorf("%w: %s: the code was issued to another apple account", ErrAppleAccountMismatch, appleExchangeStep)
	}
	return nil
}

// readExchangeIDToken decodes the payload of a JWT without looking at its header or
// signature. The decoding errors are dropped on purpose: their text can quote the token.
func readExchangeIDToken(raw string) (exchangeIDTokenClaims, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return exchangeIDTokenClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return exchangeIDTokenClaims{}, false
	}
	var claims exchangeIDTokenClaims
	if json.Unmarshal(payload, &claims) != nil {
		return exchangeIDTokenClaims{}, false
	}
	return claims, true
}

// appleAnswer is an HTTP answer of Apple with its body cut at appleResponseMaxBytes.
type appleAnswer struct {
	status int
	body   []byte
}

func (c *appleRevokeClient) post(ctx context.Context, step, endpoint string, form url.Values) (appleAnswer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return appleAnswer{}, fmt.Errorf("%w: %s: build request: %w", ErrAppleRevocationFailed, step, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		// The error names the URL and the cause, never the form.
		return appleAnswer{}, fmt.Errorf("%w: %s: %w", ErrAppleRevocationFailed, step, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, appleResponseMaxBytes))
	answer := appleAnswer{status: resp.StatusCode, body: body}
	if answer.ok() && readErr != nil {
		return appleAnswer{}, fmt.Errorf("%w: %s: read answer: %w", ErrAppleRevocationFailed, step, readErr)
	}
	return answer, nil
}

func (a appleAnswer) ok() bool { return a.status >= 200 && a.status < 300 }

// appleErrorValuePattern matches the error values Apple documents. Anything else in
// the field is not quoted, so no other text of the body reaches an error.
var appleErrorValuePattern = regexp.MustCompile(`^[a-z_]{1,64}$`)

// errorValue is the "error" field of a failed answer when it is a plain snake_case
// word, and empty otherwise.
func (a appleAnswer) errorValue() string {
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(a.body, &body) != nil || !appleErrorValuePattern.MatchString(body.Error) {
		return ""
	}
	return body.Error
}

// describe names the HTTP status and, when there is one, Apple's error value.
func (a appleAnswer) describe() string {
	text := "apple answered " + strconv.Itoa(a.status)
	if value := a.errorValue(); value != "" {
		text += " " + value
	}
	return text
}
