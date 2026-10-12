package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testAppleTeam         = "TSQ6DL62KG"
	testAppleKeyID        = "AB12CD34EF"
	testAppleClientID     = "com.mentorix.mobile"
	testAppleRedirect     = "https://api.mentorix.example/auth/apple/callback"
	testAppleCode         = "c1f9.AUTHORIZATION-CODE-SECRET.0001"
	testAppleRefreshToken = "REFRESH-TOKEN-SECRET-1"
	testAppleAccessToken  = "ACCESS-TOKEN-SECRET-1"
	// testOtherAppleSubject is the subject of an Apple account other than testAppleSubject.
	testOtherAppleSubject = "999999.other.apple.subject"
)

var testAppleNow = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

type appleCall struct {
	endpoint    string // "token" or "revoke"
	method      string
	contentType string
	rawQuery    string
	form        url.Values
}

// fakeApple serves the two Apple endpoints and records what it received. A nil
// token or revoke handler gives the success answer, whose id_token is a JWT of
// testAppleSubject for testAppleClientID.
type fakeApple struct {
	t      *testing.T
	server *httptest.Server
	token  http.HandlerFunc
	revoke http.HandlerFunc
	// delay holds every answer back, before the handlers run.
	delay time.Duration

	mu      sync.Mutex
	calls   []appleCall
	idToken string
}

func newFakeApple(t *testing.T) *fakeApple {
	t.Helper()
	a := &fakeApple{t: t, idToken: appleIDToken(t, answerClaims(testAppleSubject, testAppleClientID))}
	a.server = httptest.NewServer(a)
	t.Cleanup(a.server.Close)
	return a
}

// setIDToken changes the id_token of the success answer.
func (a *fakeApple) setIDToken(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.idToken = token
}

func (a *fakeApple) currentIDToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.idToken
}

// answerClaims are the claims of an Apple ID token for the given subject and audience.
func answerClaims(subject string, audience any) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": appleIssuer,
		"sub": subject,
		"aud": audience,
		"iat": testAppleNow.Unix(),
		"exp": testAppleNow.Add(10 * time.Minute).Unix(),
	}
}

// appleIDToken is an unsigned JWT. The client does not check the signature of the
// token in an exchange answer, and an unsigned token in the tests keeps that visible.
func appleIDToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("build id token: %v", err)
	}
	return raw
}

// rawJWT joins already encoded header and payload text into a three-part token.
func rawJWT(header, payload string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(header)) + "." + enc([]byte(payload)) + ".c2ln"
}

// exchangeJSON is the body of a successful exchange answer; id_token is left out when empty.
func exchangeJSON(t *testing.T, idToken string) string {
	t.Helper()
	fields := map[string]any{
		"access_token":  testAppleAccessToken,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": testAppleRefreshToken,
	}
	if idToken != "" {
		fields["id_token"] = idToken
	}
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal exchange answer: %v", err)
	}
	return string(body)
}

func (a *fakeApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var endpoint string
	var custom http.HandlerFunc
	switch r.URL.Path {
	case "/auth/token":
		endpoint, custom = "token", a.token
	case "/auth/revoke":
		endpoint, custom = "revoke", a.revoke
	default:
		http.NotFound(w, r)
		return
	}
	if a.delay > 0 {
		select {
		case <-time.After(a.delay):
		case <-r.Context().Done():
			return
		}
	}
	if err := r.ParseForm(); err != nil {
		a.t.Errorf("fake apple: parse form: %v", err)
	}
	a.mu.Lock()
	a.calls = append(a.calls, appleCall{
		endpoint:    endpoint,
		method:      r.Method,
		contentType: r.Header.Get("Content-Type"),
		rawQuery:    r.URL.RawQuery,
		form:        r.PostForm,
	})
	a.mu.Unlock()

	if custom != nil {
		custom(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	if endpoint == "token" {
		_, _ = io.WriteString(w, exchangeJSON(a.t, a.currentIDToken()))
	}
}

func (a *fakeApple) callsTo(endpoint string) []appleCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []appleCall
	for _, c := range a.calls {
		if c.endpoint == endpoint {
			out = append(out, c)
		}
	}
	return out
}

// clientSecrets are the client secrets Apple received, which no error may repeat.
func (a *fakeApple) clientSecrets() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, c := range a.calls {
		if s := c.form.Get("client_secret"); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func answer(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func newTestRevoker(t *testing.T, a *fakeApple) (*appleRevokeClient, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	c := NewAppleRevoker(testAppleTeam, testAppleKeyID, key).(*appleRevokeClient)
	c.tokenURL = a.server.URL + "/auth/token"
	c.revokeURL = a.server.URL + "/auth/revoke"
	c.now = func() time.Time { return testAppleNow }
	return c, key
}

func revokeWithTestCode(c *appleRevokeClient) error {
	return c.Revoke(context.Background(), testAppleClientID, testAppleSubject, testAppleCode, "")
}

// assertNoSecrets fails when the error text repeats the code, a token, a client
// secret, a part of an ID token or a subject that went over the wire. extra names
// more values, such as an ID token a test served.
func assertNoSecrets(t *testing.T, err error, a *fakeApple, extra ...string) {
	t.Helper()
	secrets := append([]string{testAppleCode, testAppleRefreshToken, testAppleAccessToken, testAppleSubject, testOtherAppleSubject}, a.clientSecrets()...)
	for _, token := range append([]string{a.currentIDToken()}, extra...) {
		secrets = append(secrets, token)
		for _, part := range strings.Split(token, ".") {
			if len(part) >= 8 {
				secrets = append(secrets, part)
			}
		}
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(err.Error(), secret) {
			t.Errorf("error text repeats a secret %q...: %q", secret[:min(len(secret), 12)], err)
		}
	}
}

func jwtPart(t *testing.T, token string, index int) (raw []byte, fields map[string]json.RawMessage) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("client secret has %d parts, want 3: %q", len(parts), token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[index])
	if err != nil {
		t.Fatalf("decode part %d: %v", index, err)
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("part %d is not a JSON object: %v", index, err)
	}
	return raw, fields
}

func TestAppleRevoker_Revoke_exchangesTheCodeThenRevokesTheRefreshToken(t *testing.T) {
	a := newFakeApple(t)
	c, _ := newTestRevoker(t, a)

	if err := revokeWithTestCode(c); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	if len(a.calls) != 2 || a.calls[0].endpoint != "token" || a.calls[1].endpoint != "revoke" {
		t.Fatalf("calls = %+v, want the exchange and then the revocation, once each", a.calls)
	}
	exchange, revoke := a.calls[0], a.calls[1]
	for _, call := range a.calls {
		if call.method != http.MethodPost {
			t.Errorf("%s method = %s, want POST", call.endpoint, call.method)
		}
		if call.contentType != "application/x-www-form-urlencoded" {
			t.Errorf("%s content type = %q, want application/x-www-form-urlencoded", call.endpoint, call.contentType)
		}
		if call.rawQuery != "" {
			t.Errorf("%s query = %q, want none: the secrets belong in the body", call.endpoint, call.rawQuery)
		}
	}

	secret := exchange.form.Get("client_secret")
	if secret == "" {
		t.Fatal("exchange has no client_secret")
	}
	wantExchange := url.Values{
		"client_id":     {testAppleClientID},
		"client_secret": {secret},
		"code":          {testAppleCode},
		"grant_type":    {"authorization_code"},
	}
	if !reflect.DeepEqual(exchange.form, wantExchange) {
		t.Errorf("exchange fields = %v, want %v", exchange.form, wantExchange)
	}
	wantRevoke := url.Values{
		"client_id":       {testAppleClientID},
		"client_secret":   {revoke.form.Get("client_secret")},
		"token":           {testAppleRefreshToken},
		"token_type_hint": {"refresh_token"},
	}
	if !reflect.DeepEqual(revoke.form, wantRevoke) {
		t.Errorf("revocation fields = %v, want %v", revoke.form, wantRevoke)
	}
	if revoke.form.Get("client_secret") == "" {
		t.Error("revocation has no client_secret")
	}
}

func TestAppleRevoker_Revoke_sendsTheRedirectURIOnlyWhenGiven(t *testing.T) {
	tests := []struct {
		name        string
		redirectURI string
		want        []string
	}{
		{"android web flow", testAppleRedirect, []string{testAppleRedirect}},
		{"native ios flow", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeApple(t)
			c, _ := newTestRevoker(t, a)

			if err := c.Revoke(context.Background(), testAppleClientID, testAppleSubject, testAppleCode, tt.redirectURI); err != nil {
				t.Fatalf("Revoke() error = %v", err)
			}

			exchange, revoke := a.callsTo("token")[0], a.callsTo("revoke")[0]
			got, present := exchange.form["redirect_uri"]
			if !reflect.DeepEqual(got, tt.want) || present != (tt.want != nil) {
				t.Errorf("exchange redirect_uri = %v (present %v), want %v", got, present, tt.want)
			}
			if _, present := revoke.form["redirect_uri"]; present {
				t.Error("the revocation carries redirect_uri")
			}
		})
	}
}

func TestAppleRevoker_Revoke_clientSecretIsASignedES256JWT(t *testing.T) {
	a := newFakeApple(t)
	c, key := newTestRevoker(t, a)

	if err := revokeWithTestCode(c); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	for _, call := range a.calls {
		secret := call.form.Get("client_secret")
		parsed, err := jwt.Parse(secret, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
			jwt.WithValidMethods([]string{"ES256"}), jwt.WithTimeFunc(func() time.Time { return testAppleNow }))
		if err != nil || !parsed.Valid {
			t.Fatalf("%s client secret does not verify with the public key: %v", call.endpoint, err)
		}

		_, header := jwtPart(t, secret, 0)
		if string(header["alg"]) != `"ES256"` || string(header["kid"]) != `"`+testAppleKeyID+`"` {
			t.Errorf("%s header = %v, want alg ES256 and kid %s", call.endpoint, header, testAppleKeyID)
		}
		// The payload is read as JSON, not through a claims type, so the form of aud shows.
		_, claims := jwtPart(t, secret, 1)
		want := map[string]string{
			"iss": `"` + testAppleTeam + `"`,
			"sub": `"` + testAppleClientID + `"`,
			"aud": `"https://appleid.apple.com"`,
			"iat": "1791795600",
			"exp": "1791795900",
		}
		if len(claims) != len(want) {
			t.Errorf("%s claims = %v, want exactly %d", call.endpoint, claims, len(want))
		}
		for name, wantValue := range want {
			if string(claims[name]) != wantValue {
				t.Errorf("%s claim %s = %s, want %s", call.endpoint, name, claims[name], wantValue)
			}
		}
		if got := call.form.Get("client_id"); got != testAppleClientID {
			t.Errorf("%s client_id = %q, want the sub of the secret %q", call.endpoint, got, testAppleClientID)
		}
	}
	if iat := testAppleNow.Unix(); iat != 1791795600 {
		t.Fatalf("the fixed clock is %d, the expected claims above assume 1791795600", iat)
	}
}

func TestAppleRevoker_Revoke_clientSecretIsForTheGivenClientID(t *testing.T) {
	a := newFakeApple(t)
	c, _ := newTestRevoker(t, a)

	for _, clientID := range []string{"com.mentorix.mobile", "com.mentorix.service"} {
		a.setIDToken(appleIDToken(t, answerClaims(testAppleSubject, clientID)))
		if err := c.Revoke(context.Background(), clientID, testAppleSubject, testAppleCode, ""); err != nil {
			t.Fatalf("Revoke(%s) error = %v", clientID, err)
		}
	}

	if len(a.calls) != 4 {
		t.Fatalf("calls = %d, want 4", len(a.calls))
	}
	for i, call := range a.calls {
		wantID := []string{"com.mentorix.mobile", "com.mentorix.service"}[i/2]
		_, claims := jwtPart(t, call.form.Get("client_secret"), 1)
		if string(claims["sub"]) != `"`+wantID+`"` || call.form.Get("client_id") != wantID {
			t.Errorf("call %d: sub = %s, client_id = %q, want %s", i, claims["sub"], call.form.Get("client_id"), wantID)
		}
	}
}

func TestAppleRevoker_Revoke_exchangeFailures(t *testing.T) {
	huge := `{"refresh_token":"` + testAppleRefreshToken + `","pad":"` + strings.Repeat("a", appleResponseMaxBytes) + `"}`
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		want     error
		wantText []string
	}{
		{"invalid_grant", answer(400, `{"error":"invalid_grant"}`), ErrInvalidAppleAuthorizationCode, []string{"token exchange", "invalid_grant"}},
		{"invalid_grant with a description", answer(400, `{"error":"invalid_grant","error_description":"code `+testAppleCode+` is used"}`),
			ErrInvalidAppleAuthorizationCode, []string{"invalid_grant"}},
		{"invalid_client", answer(400, `{"error":"invalid_client"}`), ErrAppleRevocationFailed, []string{"token exchange", "invalid_client"}},
		{"invalid_request", answer(400, `{"error":"invalid_request"}`), ErrAppleRevocationFailed, []string{"invalid_request"}},
		{"unauthorized_client", answer(400, `{"error":"unauthorized_client"}`), ErrAppleRevocationFailed, []string{"unauthorized_client"}},
		{"unsupported_grant_type", answer(400, `{"error":"unsupported_grant_type"}`), ErrAppleRevocationFailed, []string{"unsupported_grant_type"}},
		{"invalid_scope", answer(400, `{"error":"invalid_scope"}`), ErrAppleRevocationFailed, []string{"invalid_scope"}},
		{"an error value that is not a plain word is not echoed", answer(400, `{"error":"Bad `+testAppleCode+` sent"}`),
			ErrAppleRevocationFailed, []string{"token exchange", "400"}},
		{"500 with an html page", answer(500, `<html>`+testAppleCode+`</html>`), ErrAppleRevocationFailed, []string{"token exchange", "500"}},
		{"500 with an empty body", answer(500, ``), ErrAppleRevocationFailed, []string{"500"}},
		{"400 with a body that is not json", answer(400, `plain text `+testAppleCode), ErrAppleRevocationFailed, []string{"400"}},
		{"400 whose error is not a string", answer(400, `{"error":["invalid_grant"]}`), ErrAppleRevocationFailed, []string{"400"}},
		{"200 without a refresh token", answer(200, `{"access_token":"`+testAppleAccessToken+`","id_token":"`+appleIDToken(t, answerClaims(testAppleSubject, testAppleClientID))+`","token_type":"Bearer","expires_in":3600}`),
			ErrAppleRevocationFailed, []string{"token exchange", "refresh token"}},
		{"200 with an empty refresh token", answer(200, `{"access_token":"`+testAppleAccessToken+`","refresh_token":""}`), ErrAppleRevocationFailed, []string{"refresh token"}},
		{"200 with a refresh token that is not a string", answer(200, `{"refresh_token":12345}`), ErrAppleRevocationFailed, []string{"refresh token"}},
		{"200 that is not json", answer(200, `not json `+testAppleAccessToken), ErrAppleRevocationFailed, []string{"refresh token"}},
		{"200 with an empty body", answer(200, ``), ErrAppleRevocationFailed, []string{"refresh token"}},
		{"200 whose body is longer than the limit", answer(200, huge), ErrAppleRevocationFailed, []string{"refresh token"}},
		{"a redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/somewhere-else", http.StatusTemporaryRedirect)
		}, ErrAppleRevocationFailed, []string{"token exchange", "307"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeApple(t)
			a.token = tt.handler
			c, _ := newTestRevoker(t, a)

			err := revokeWithTestCode(c)

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			for _, other := range []error{ErrInvalidAppleAuthorizationCode, ErrAppleRevocationFailed} {
				if other != tt.want && errors.Is(err, other) {
					t.Errorf("error %v is also %v, the two sentinels must differ", err, other)
				}
			}
			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			assertNoSecrets(t, err, a)
			if n := len(a.callsTo("revoke")); n != 0 {
				t.Errorf("the revocation was called %d times after a failed exchange", n)
			}
		})
	}
}

func TestAppleRevoker_Revoke_revocationFailures(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantText []string
	}{
		{"invalid_client", answer(400, `{"error":"invalid_client"}`), []string{"token revocation", "invalid_client"}},
		// invalid_grant means a bad code only on the exchange.
		{"invalid_grant", answer(400, `{"error":"invalid_grant"}`), []string{"token revocation", "invalid_grant"}},
		{"invalid_request", answer(400, `{"error":"invalid_request","error_description":"token `+testAppleRefreshToken+`"}`), []string{"invalid_request"}},
		{"500", answer(500, `server says `+testAppleRefreshToken), []string{"token revocation", "500"}},
		{"503 without a body", answer(503, ``), []string{"503"}},
		{"a redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/somewhere-else", http.StatusFound)
		}, []string{"token revocation", "302"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeApple(t)
			a.revoke = tt.handler
			c, _ := newTestRevoker(t, a)

			err := revokeWithTestCode(c)

			if !errors.Is(err, ErrAppleRevocationFailed) {
				t.Fatalf("error = %v, want ErrAppleRevocationFailed", err)
			}
			if errors.Is(err, ErrInvalidAppleAuthorizationCode) {
				t.Errorf("error %v is also ErrInvalidAppleAuthorizationCode", err)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			assertNoSecrets(t, err, a)
			if n := len(a.callsTo("token")); n != 1 {
				t.Errorf("exchange calls = %d, want 1", n)
			}
		})
	}
}

func TestAppleRevoker_Revoke_acceptsAnyBodyOnASuccessfulRevocation(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"200 and an empty body": nil,
		"200 and a json body":   answer(200, `{}`),
		"204":                   answer(204, ``),
	} {
		t.Run(name, func(t *testing.T) {
			a := newFakeApple(t)
			a.revoke = handler
			c, _ := newTestRevoker(t, a)

			if err := revokeWithTestCode(c); err != nil {
				t.Fatalf("Revoke() error = %v", err)
			}
		})
	}
}

// The forms hold the code and the tokens: a redirect must never carry them to
// another address.
func TestAppleRevoker_Revoke_doesNotFollowRedirects(t *testing.T) {
	for _, endpoint := range []string{"token", "revoke"} {
		t.Run(endpoint, func(t *testing.T) {
			var mu sync.Mutex
			hits := 0
			elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				mu.Lock()
				hits++
				mu.Unlock()
			}))
			t.Cleanup(elsewhere.Close)
			a := newFakeApple(t)
			redirect := func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, elsewhere.URL+"/collect", http.StatusTemporaryRedirect)
			}
			if endpoint == "token" {
				a.token = redirect
			} else {
				a.revoke = redirect
			}
			c, _ := newTestRevoker(t, a)

			err := revokeWithTestCode(c)

			if !errors.Is(err, ErrAppleRevocationFailed) {
				t.Fatalf("error = %v, want ErrAppleRevocationFailed", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if hits != 0 {
				t.Fatalf("the redirect target received %d requests, want 0", hits)
			}
		})
	}
}

func TestAppleRevoker_Revoke_timesOut(t *testing.T) {
	hang := func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}
	for _, endpoint := range []string{"token", "revoke"} {
		t.Run(endpoint, func(t *testing.T) {
			a := newFakeApple(t)
			if endpoint == "token" {
				a.token = hang
			} else {
				a.revoke = hang
			}
			c, _ := newTestRevoker(t, a)
			c.client.Timeout = 100 * time.Millisecond

			start := time.Now()
			err := revokeWithTestCode(c)

			if !errors.Is(err, ErrAppleRevocationFailed) {
				t.Fatalf("error = %v, want ErrAppleRevocationFailed", err)
			}
			var netErr net.Error
			if !errors.As(err, &netErr) || !netErr.Timeout() {
				t.Errorf("error %v does not keep the timeout as its cause", err)
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Errorf("took %v with a 100ms timeout", elapsed)
			}
			assertNoSecrets(t, err, a)
		})
	}
}

func TestAppleRevoker_Revoke_transportErrorKeepsSecretsOut(t *testing.T) {
	a := newFakeApple(t)
	c, _ := newTestRevoker(t, a)
	// A first, successful call gives the fake a client secret to look for.
	if err := revokeWithTestCode(c); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	a.server.Close()

	err := revokeWithTestCode(c)

	if !errors.Is(err, ErrAppleRevocationFailed) {
		t.Fatalf("error = %v, want ErrAppleRevocationFailed", err)
	}
	if !strings.Contains(err.Error(), "token exchange") {
		t.Errorf("error %q does not name the step", err)
	}
	assertNoSecrets(t, err, a)
}

func TestAppleRevoker_Revoke_cancelledContextSendsNothing(t *testing.T) {
	a := newFakeApple(t)
	c, _ := newTestRevoker(t, a)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Revoke(ctx, testAppleClientID, testAppleSubject, testAppleCode, "")

	if !errors.Is(err, ErrAppleRevocationFailed) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want ErrAppleRevocationFailed caused by context.Canceled", err)
	}
	if len(a.calls) != 0 {
		t.Errorf("calls = %+v, want none", a.calls)
	}
}

func TestAppleRevoker_Revoke_readsOnlyALimitedAnswer(t *testing.T) {
	a := newFakeApple(t)
	var mu sync.Mutex
	var written int
	a.token = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		chunk := []byte(strings.Repeat("a", 32<<10))
		_, _ = w.Write([]byte(`{"pad":"`))
		for i := 0; i < 2048; i++ { // up to 64 MiB, if the client keeps reading
			n, err := w.Write(chunk)
			mu.Lock()
			written += n
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}
	c, _ := newTestRevoker(t, a)

	err := revokeWithTestCode(c)

	if !errors.Is(err, ErrAppleRevocationFailed) {
		t.Fatalf("error = %v, want ErrAppleRevocationFailed", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if written >= 32<<20 {
		t.Errorf("the server wrote %d bytes: the client kept reading far past its limit", written)
	}
}

func TestAppleRevoker_Revoke_isSafeForConcurrentUse(t *testing.T) {
	a := newFakeApple(t)
	c, _ := newTestRevoker(t, a)
	var wg sync.WaitGroup
	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- revokeWithTestCode(c)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("Revoke() error = %v", err)
		}
	}
}

func TestNewAppleRevoker_usesAppleEndpointsWithABoundedClient(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	c := NewAppleRevoker(testAppleTeam, testAppleKeyID, key).(*appleRevokeClient)

	if c.tokenURL != "https://appleid.apple.com/auth/token" || c.revokeURL != "https://appleid.apple.com/auth/revoke" {
		t.Errorf("urls = %q and %q, want Apple's token and revoke endpoints", c.tokenURL, c.revokeURL)
	}
	if c.client.Timeout != 10*time.Second || c.budget != 10*time.Second {
		t.Errorf("client timeout = %v, budget = %v, want 10s for both", c.client.Timeout, c.budget)
	}
	if c.client.CheckRedirect == nil || !errors.Is(c.client.CheckRedirect(nil, nil), http.ErrUseLastResponse) {
		t.Error("the client follows redirects")
	}
	if c.now == nil || time.Since(c.now()) > time.Minute {
		t.Error("the clock is not the wall clock")
	}
}

// The code must belong to the Apple account the app proved. The exchange answer carries
// an ID token of the account the code was issued to; its subject has to be the expected
// one before anything is revoked.
func TestAppleRevoker_Revoke_acceptsAnIDTokenOfTheExpectedAccountAndClient(t *testing.T) {
	tests := []struct {
		name     string
		audience any
	}{
		{"audience as a string", testAppleClientID},
		{"audience as an array", []string{testAppleClientID}},
		{"audience as an array with other values", []string{"com.mentorix.other", testAppleClientID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeApple(t)
			a.setIDToken(appleIDToken(t, answerClaims(testAppleSubject, tt.audience)))
			c, _ := newTestRevoker(t, a)

			if err := revokeWithTestCode(c); err != nil {
				t.Fatalf("Revoke() error = %v", err)
			}

			if n := len(a.callsTo("revoke")); n != 1 {
				t.Errorf("revocation calls = %d, want 1", n)
			}
		})
	}
}

func TestAppleRevoker_Revoke_aCodeOfAnotherAppleAccountRevokesNothing(t *testing.T) {
	for name, audience := range map[string]any{"string audience": testAppleClientID, "array audience": []string{testAppleClientID}} {
		t.Run(name, func(t *testing.T) {
			a := newFakeApple(t)
			token := appleIDToken(t, answerClaims(testOtherAppleSubject, audience))
			a.setIDToken(token)
			c, _ := newTestRevoker(t, a)

			err := revokeWithTestCode(c)

			if !errors.Is(err, ErrAppleAccountMismatch) {
				t.Fatalf("error = %v, want ErrAppleAccountMismatch", err)
			}
			if errors.Is(err, ErrAppleRevocationFailed) || errors.Is(err, ErrInvalidAppleAuthorizationCode) {
				t.Errorf("error %v is also another sentinel", err)
			}
			if !strings.Contains(err.Error(), "token exchange") {
				t.Errorf("error %q does not name the step", err)
			}
			if n := len(a.callsTo("revoke")); n != 0 {
				t.Errorf("the revocation was called %d times for the wrong account", n)
			}
			assertNoSecrets(t, err, a, token)
		})
	}
}

func TestAppleRevoker_Revoke_anIDTokenThatCannotBeTrustedFailsWithoutRevoking(t *testing.T) {
	bad := func(mutate func(c jwt.MapClaims)) string {
		claims := answerClaims(testAppleSubject, testAppleClientID)
		mutate(claims)
		return appleIDToken(t, claims)
	}
	const header = `{"alg":"none","typ":"JWT"}`
	validParts := strings.Split(appleIDToken(t, answerClaims(testAppleSubject, testAppleClientID)), ".")
	tests := []struct {
		name  string
		token string // empty: the answer has no id_token
	}{
		{"no id_token", ""},
		{"not a jwt", "not-a-jwt"},
		// Valid claims in the second part: only the part count tells these from a good token.
		{"valid claims, two parts", validParts[0] + "." + validParts[1]},
		{"valid claims, four parts", validParts[0] + "." + validParts[1] + ".c2ln.ZXh0cmE"},
		{"payload that is not base64", "eyJhbGciOiJub25lIn0.!!!not-base64!!!.c2ln"},
		{"payload that is not json", rawJWT(header, "this is not json")},
		{"payload that is an array", rawJWT(header, `["sub","aud"]`)},
		{"payload that is a string", rawJWT(header, `"sub"`)},
		{"another issuer", bad(func(c jwt.MapClaims) { c["iss"] = "https://accounts.google.com" })},
		{"a look-alike issuer", bad(func(c jwt.MapClaims) { c["iss"] = "https://appleid.apple.com.evil.example" })},
		{"no issuer", bad(func(c jwt.MapClaims) { delete(c, "iss") })},
		{"another audience", bad(func(c jwt.MapClaims) { c["aud"] = "com.mentorix.other" })},
		{"an array without the client id", bad(func(c jwt.MapClaims) { c["aud"] = []string{"com.mentorix.other", "com.mentorix.third"} })},
		{"an empty audience array", bad(func(c jwt.MapClaims) { c["aud"] = []string{} })},
		{"no audience", bad(func(c jwt.MapClaims) { delete(c, "aud") })},
		{"an audience of the wrong type", bad(func(c jwt.MapClaims) { c["aud"] = 5 })},
		{"no subject", bad(func(c jwt.MapClaims) { delete(c, "sub") })},
		{"an empty subject", bad(func(c jwt.MapClaims) { c["sub"] = "" })},
		{"a subject of the wrong type", bad(func(c jwt.MapClaims) { c["sub"] = 12345 })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeApple(t)
			a.token = answer(200, exchangeJSON(t, tt.token))
			c, _ := newTestRevoker(t, a)

			err := revokeWithTestCode(c)

			if !errors.Is(err, ErrAppleRevocationFailed) {
				t.Fatalf("error = %v, want ErrAppleRevocationFailed", err)
			}
			if errors.Is(err, ErrAppleAccountMismatch) || errors.Is(err, ErrInvalidAppleAuthorizationCode) {
				t.Errorf("error %v is also another sentinel", err)
			}
			if !strings.Contains(err.Error(), "token exchange") || !strings.Contains(err.Error(), "id_token") {
				t.Errorf("error %q does not name the step and the id_token", err)
			}
			if n := len(a.callsTo("revoke")); n != 0 {
				t.Errorf("the revocation was called %d times after an id_token that cannot be trusted", n)
			}
			assertNoSecrets(t, err, a, tt.token)
		})
	}
}

// Revoke makes two calls that each get the client timeout; one budget covers both, so
// the whole step ends inside what the server's write timeout allows.
func TestAppleRevoker_Revoke_oneBudgetCoversBothCalls(t *testing.T) {
	tests := []struct {
		name    string
		budget  time.Duration
		wantErr bool
	}{
		{"a budget that fits both calls", 2 * time.Second, false},
		{"a budget that fits one call but not both", 200 * time.Millisecond, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newFakeApple(t)
			a.delay = 150 * time.Millisecond
			c, _ := newTestRevoker(t, a)
			c.budget = tt.budget

			err := revokeWithTestCode(c)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Revoke() error = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrAppleRevocationFailed) {
				t.Fatalf("error = %v, want ErrAppleRevocationFailed", err)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("error %v does not keep the deadline as its cause", err)
			}
			if n := len(a.callsTo("token")); n != 1 {
				t.Errorf("exchange calls = %d, want 1", n)
			}
			assertNoSecrets(t, err, a)
		})
	}
}
