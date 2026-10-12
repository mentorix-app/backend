package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
)

func (f *fakeAuthService) DeleteAccount(_ context.Context, userID uuid.UUID, proof AppleProof) error {
	f.deleteCalls = append(f.deleteCalls, userID)
	f.deleteProofs = append(f.deleteProofs, proof)
	return f.deleteErr
}

// deleteMe sends DELETE /auth/me with body as JSON, or without a body when it is empty.
func deleteMe(t *testing.T, e *echo.Echo, authorization, body string) *httptest.ResponseRecorder {
	t.Helper()
	return deleteMeAt(t, e, "/auth/me", authorization, body)
}

func deleteMeAt(t *testing.T, e *echo.Echo, target, authorization, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if authorization != "" {
		req.Header.Set(echo.HeaderAuthorization, authorization)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestHandlers_Mount_registersDeleteMeRoute(t *testing.T) {
	e := echo.New()
	testAuthHandlers(&fakeAuthService{}).Mount(e)
	for _, r := range e.Routes() {
		if r.Method == http.MethodDelete && r.Path == "/auth/me" {
			return
		}
	}
	t.Fatal("missing route DELETE /auth/me")
}

func TestDeleteMe_success(t *testing.T) {
	userID := uuid.New()
	svc := &fakeAuthService{}
	e := codedEcho(testAuthHandlers(svc))

	rec := deleteMe(t, e, bearerToken(t, userID), "")

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body)
	}
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
	if len(svc.deleteCalls) != 1 || svc.deleteCalls[0] != userID {
		t.Errorf("service called with %v, want exactly [%v]", svc.deleteCalls, userID)
	}
}

func TestDeleteMe_requiresToken(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		wantCode      string
		wantMessage   string
	}{
		{"no header", "", "missing_authorization", httpx.MsgMissingAuth},
		{"garbage token", "Bearer not-a-jwt", "invalid_token", httpx.MsgInvalidToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}

			rec := deleteMe(t, codedEcho(testAuthHandlers(svc)), tt.authorization, "")

			assertCodedError(t, rec, http.StatusUnauthorized, tt.wantCode, tt.wantMessage)
			if len(svc.deleteCalls) != 0 {
				t.Error("service must not be called without a valid token")
			}
		})
	}
}

func TestDeleteMe_errorAnswers(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"account gone", pgx.ErrNoRows, 404, "user_not_found", httpx.MsgUserNotFound},
		{"account gone, wrapped", fmt.Errorf("delete account: %w", pgx.ErrNoRows), 404, "user_not_found", httpx.MsgUserNotFound},
		{"deletion unavailable", ErrAccountDeletionUnavailable, 409, "account_deletion_unavailable", "this account cannot be deleted in the app"},
		{"deletion unavailable, wrapped", fmt.Errorf("delete account: %w", ErrAccountDeletionUnavailable), 409, "account_deletion_unavailable", "this account cannot be deleted in the app"},
		{"apple not configured", ErrProviderNotConfigured, 503, "provider_not_configured", "apple account deletion is not configured"},
		{"apple not configured, wrapped", fmt.Errorf("delete account: %w", ErrProviderNotConfigured), 503, "provider_not_configured", "apple account deletion is not configured"},
		{"apple authorization required", ErrAppleAuthorizationRequired, 422, "apple_authorization_required", "apple authorization is required"},
		{"apple authorization required, wrapped", fmt.Errorf("delete account: %w", ErrAppleAuthorizationRequired), 422, "apple_authorization_required", "apple authorization is required"},
		{"invalid id token", fmt.Errorf("%w: token is expired", ErrInvalidIDToken), 401, "invalid_id_token", "invalid id token"},
		{"apple account mismatch", ErrAppleAccountMismatch, 403, "apple_account_mismatch", "this apple account does not belong to the account"},
		{"apple account mismatch, wrapped", fmt.Errorf("delete account: %w", ErrAppleAccountMismatch), 403, "apple_account_mismatch", "this apple account does not belong to the account"},
		{"invalid authorization code", fmt.Errorf("%w: token exchange: apple answered 400 invalid_grant", ErrInvalidAppleAuthorizationCode), 400, "invalid_apple_authorization_code", "invalid or expired apple authorization code"},
		{"apple revocation failed", fmt.Errorf("%w: token exchange: apple answered 400 invalid_client", ErrAppleRevocationFailed), 502, "apple_revocation_failed", "apple did not confirm the revocation"},
		{"anything else", errors.New("db down"), 500, "internal_error", "account deletion failed"},
		// The service maps this one to ErrProviderNotConfigured; if it ever arrives
		// unmapped the handler fails closed instead of guessing a status.
		{"apple not revoked, unmapped", ErrAppleSignInNotRevoked, 500, "internal_error", "account deletion failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := codedEcho(testAuthHandlers(&fakeAuthService{deleteErr: tt.err}))

			rec := deleteMe(t, e, bearerToken(t, uuid.New()), "")

			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
			if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
				t.Errorf("Set-Cookie = %v, want none", got)
			}
		})
	}
}

// A refusal and a failure keep their cause for the request log, and none of it
// reaches the body.
func TestDeleteMe_causeReachesTheLogNotTheBody(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantLog []string
	}{
		{"internal error", errors.New("db down at 10.0.0.5"), []string{"db down at 10.0.0.5"}},
		{"refused by the database", fmt.Errorf("%w: violates foreign key programs_created_by_fkey", ErrAccountDeletionUnavailable),
			[]string{"account_deletion_unavailable", "programs_created_by_fkey"}},
		{"invalid id token", fmt.Errorf("%w: oidc: token is expired (Token Expiry: 2026-01-01)", ErrInvalidIDToken),
			[]string{"invalid_id_token", "Token Expiry: 2026-01-01"}},
		{"invalid authorization code", fmt.Errorf("%w: token exchange: apple answered 400 invalid_grant", ErrInvalidAppleAuthorizationCode),
			[]string{"invalid_apple_authorization_code", "apple answered 400 invalid_grant"}},
		{"apple revocation failed", fmt.Errorf("%w: token revocation: apple answered 400 invalid_client", ErrAppleRevocationFailed),
			[]string{"apple_revocation_failed", "apple answered 400 invalid_client"}},
	}
	for _, tt := range tests {
		// Echo's own handler would print the internal error with Debug on; ours must not.
		for _, debug := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, debug %v", tt.name, debug), func(t *testing.T) {
				var logs bytes.Buffer
				e := codedEcho(testAuthHandlers(&fakeAuthService{deleteErr: tt.err}),
					httpx.RequestLog(slog.New(slog.NewTextHandler(&logs, nil))))
				e.Debug = debug

				rec := deleteMe(t, e, bearerToken(t, uuid.New()), "")

				if rec.Code < 400 {
					t.Fatalf("status = %d, want an error", rec.Code)
				}
				for _, leaked := range []string{"db down", "10.0.0.5", "programs_created_by_fkey", "foreign key"} {
					if strings.Contains(rec.Body.String(), leaked) {
						t.Errorf("body leaks %q: %s", leaked, rec.Body)
					}
				}
				for _, want := range tt.wantLog {
					if !strings.Contains(logs.String(), want) {
						t.Errorf("log %q does not contain %q", logs.String(), want)
					}
				}
			})
		}
	}
}

// The refresh cookie is not a credential for this endpoint: only the Bearer token is.
func TestDeleteMe_refreshCookieAloneIsNotAuthentication(t *testing.T) {
	svc := &fakeAuthService{}
	e := codedEcho(testAuthHandlers(svc))
	req := httptest.NewRequest(http.MethodDelete, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "a-valid-looking-refresh-token"})
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assertCodedError(t, rec, http.StatusUnauthorized, "missing_authorization", httpx.MsgMissingAuth)
	if len(svc.deleteCalls) != 0 {
		t.Error("service must not be called with only a refresh cookie")
	}
}

// Only the id in the token counts: neither a JSON body nor the query string can
// name another account.
func TestDeleteMe_deletesOnlyTheIDFromTheToken(t *testing.T) {
	owner, other := uuid.New(), uuid.New()
	svc := &fakeAuthService{}
	e := codedEcho(testAuthHandlers(svc))
	req := httptest.NewRequest(http.MethodDelete, "/auth/me?user_id="+other.String(), strings.NewReader(`{"user_id":"`+other.String()+`"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, bearerToken(t, owner))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.deleteCalls) != 1 || svc.deleteCalls[0] != owner {
		t.Fatalf("service called with %v, want exactly [%v] (the token's user, not %v)", svc.deleteCalls, owner, other)
	}
}

const (
	secretIDToken = "APP-ID-TOKEN-SECRET-456"
	secretCode    = "APP-CODE-SECRET-123"
	redirectURI   = "https://api.mentorix.example/auth/apple/callback"
)

func deleteBody(idToken, code, redirect string) string {
	return fmt.Sprintf(`{"apple_id_token":%q,"apple_authorization_code":%q,"apple_redirect_uri":%q}`, idToken, code, redirect)
}

func TestDeleteMe_passesTheAppleProofFromTheBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want AppleProof
	}{
		{"all three fields", deleteBody(secretIDToken, secretCode, redirectURI), AppleProof{IDToken: secretIDToken, AuthorizationCode: secretCode, RedirectURI: redirectURI}},
		{"without a redirect address", `{"apple_id_token":"t","apple_authorization_code":"c"}`, AppleProof{IDToken: "t", AuthorizationCode: "c"}},
		{"an empty redirect address", `{"apple_id_token":"t","apple_authorization_code":"c","apple_redirect_uri":""}`, AppleProof{IDToken: "t", AuthorizationCode: "c"}},
		{"unknown fields are ignored", `{"apple_id_token":"t","apple_authorization_code":"c","user_id":"x","extra":1}`, AppleProof{IDToken: "t", AuthorizationCode: "c"}},
		{"no body", "", AppleProof{}},
		{"an empty object", `{}`, AppleProof{}},
		{"null", `null`, AppleProof{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			userID := uuid.New()

			rec := deleteMe(t, codedEcho(testAuthHandlers(svc)), bearerToken(t, userID), tt.body)

			assertHTTPStatus(t, rec, http.StatusNoContent)
			if len(svc.deleteCalls) != 1 || svc.deleteCalls[0] != userID || svc.deleteProofs[0] != tt.want {
				t.Errorf("service calls = %v with proofs %+v, want one for %v with %+v", svc.deleteCalls, svc.deleteProofs, userID, tt.want)
			}
		})
	}
}

// A body that is absent or empty is valid, however the client frames it.
func TestDeleteMe_anEmptyBodyIsValidInEveryFraming(t *testing.T) {
	tests := []struct {
		name        string
		body        io.Reader
		contentType string
	}{
		{"no body, no content type", nil, ""},
		{"empty body, json content type", strings.NewReader(""), echo.MIMEApplicationJSON},
		{"whitespace body, json content type", strings.NewReader(" \n\t "), echo.MIMEApplicationJSON},
		// An unknown length, as a chunked request has it.
		{"empty chunked body, json content type", io.NopCloser(strings.NewReader("")), echo.MIMEApplicationJSON},
		{"json with a charset", strings.NewReader(`{}`), echo.MIMEApplicationJSON + "; charset=utf-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			req := httptest.NewRequest(http.MethodDelete, "/auth/me", tt.body)
			if tt.contentType != "" {
				req.Header.Set(echo.HeaderContentType, tt.contentType)
			}
			req.Header.Set(echo.HeaderAuthorization, bearerToken(t, uuid.New()))
			rec := httptest.NewRecorder()

			codedEcho(testAuthHandlers(svc)).ServeHTTP(rec, req)

			assertHTTPStatus(t, rec, http.StatusNoContent)
			if len(svc.deleteProofs) != 1 || svc.deleteProofs[0] != (AppleProof{}) {
				t.Errorf("proofs = %+v, want one empty proof", svc.deleteProofs)
			}
		})
	}
}

func TestDeleteMe_invalidJSONIsABadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"not json", "not-json"},
		{"truncated", `{"apple_id_token":"` + secretIDToken + `",`},
		{"an array", `[]`},
		{"a string", `"apple"`},
		{"a field of the wrong type", `{"apple_id_token":5,"apple_authorization_code":"c"}`},
		{"two objects", `{} {}`},
		{"trailing text", `{} tail`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}

			rec := deleteMe(t, codedEcho(testAuthHandlers(svc)), bearerToken(t, uuid.New()), tt.body)

			assertCodedError(t, rec, 400, "bad_request", httpx.MsgInvalidJSON)
			if len(svc.deleteCalls) != 0 {
				t.Error("service must not be called with an invalid body")
			}
		})
	}
}

// deleteMeTyped sends body with exactly the given Content-Type, or none when it is empty.
func deleteMeTyped(t *testing.T, e *echo.Echo, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/auth/me", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set(echo.HeaderContentType, contentType)
	}
	req.Header.Set(echo.HeaderAuthorization, bearerToken(t, uuid.New()))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// A body is JSON by its Content-Type or it is refused, even when the text happens to be
// valid JSON: the header is what the app promises, and a guess would accept forms.
func TestDeleteMe_aBodyNeedsTheJSONContentType(t *testing.T) {
	valid := deleteBody("t", "c", "")
	for _, contentType := range []string{
		"text/plain",
		"text/plain; charset=utf-8",
		echo.MIMEApplicationForm,
		echo.MIMEMultipartForm,
		"application/xml",
		"application/octet-stream",
		"application/jsonx",
		"application/json-patch+json",
		"application/vnd.api+json",
		"application/json; charset",
		"json",
		"",
	} {
		t.Run("content type "+contentType, func(t *testing.T) {
			svc := &fakeAuthService{}

			rec := deleteMeTyped(t, codedEcho(testAuthHandlers(svc)), contentType, valid)

			assertCodedError(t, rec, 400, "bad_request", httpx.MsgInvalidJSON)
			if len(svc.deleteCalls) != 0 {
				t.Error("service must not be called")
			}
		})
	}
}

func TestDeleteMe_theJSONContentTypeIsReadAsAMediaType(t *testing.T) {
	for _, contentType := range []string{
		"application/json",
		"Application/JSON; charset=utf-8",
		"APPLICATION/JSON",
		"application/json;charset=UTF-8",
		"application/json ; charset=utf-8",
	} {
		t.Run(contentType, func(t *testing.T) {
			svc := &fakeAuthService{}

			rec := deleteMeTyped(t, codedEcho(testAuthHandlers(svc)), contentType, deleteBody("t", "c", ""))

			assertHTTPStatus(t, rec, http.StatusNoContent)
			if len(svc.deleteProofs) != 1 || svc.deleteProofs[0] != (AppleProof{IDToken: "t", AuthorizationCode: "c"}) {
				t.Errorf("proofs = %+v, want the body's proof", svc.deleteProofs)
			}
		})
	}
}

// An empty body needs no Content-Type, whatever header is sent.
func TestDeleteMe_anEmptyBodyNeedsNoContentType(t *testing.T) {
	for _, contentType := range []string{"", "text/plain", echo.MIMEApplicationForm} {
		svc := &fakeAuthService{}

		rec := deleteMeTyped(t, codedEcho(testAuthHandlers(svc)), contentType, "")

		if rec.Code != http.StatusNoContent {
			t.Errorf("content type %q: status = %d, want 204", contentType, rec.Code)
		}
	}
}

// The access token is checked before the body is looked at: a request without a valid
// token never gets a body-size answer.
func TestDeleteMe_aBadTokenIsAnsweredBeforeTheBodyLimit(t *testing.T) {
	big := deleteBody(strings.Repeat("a", 16000), "c", "")
	tests := []struct {
		name          string
		authorization string
		wantCode      string
		wantMessage   string
	}{
		{"no token", "", "missing_authorization", httpx.MsgMissingAuth},
		{"a bad token", "Bearer not-a-jwt", "invalid_token", httpx.MsgInvalidToken},
	}
	for _, tt := range tests {
		for name, body := range map[string]io.Reader{"known length": strings.NewReader(big), "unknown length": io.NopCloser(strings.NewReader(big))} {
			t.Run(tt.name+", "+name, func(t *testing.T) {
				svc := &fakeAuthService{}
				req := httptest.NewRequest(http.MethodDelete, "/auth/me", body)
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				if tt.authorization != "" {
					req.Header.Set(echo.HeaderAuthorization, tt.authorization)
				}
				rec := httptest.NewRecorder()

				codedEcho(testAuthHandlers(svc)).ServeHTTP(rec, req)

				assertCodedError(t, rec, http.StatusUnauthorized, tt.wantCode, tt.wantMessage)
				if len(svc.deleteCalls) != 0 {
					t.Error("service must not be called")
				}
			})
		}
	}
}

// The Apple proof is read from the JSON body only: whatever the query string holds, it
// never fills a field.
func TestDeleteMe_theQueryStringNeverFillsTheProof(t *testing.T) {
	query := "?apple_id_token=" + secretIDToken + "&apple_authorization_code=" + secretCode + "&apple_redirect_uri=" + redirectURI
	tests := []struct {
		name string
		body string
		want AppleProof
	}{
		{"no body", "", AppleProof{}},
		{"an empty object", `{}`, AppleProof{}},
		{"a body with other values", `{"apple_id_token":"body-token","apple_authorization_code":"body-code"}`, AppleProof{IDToken: "body-token", AuthorizationCode: "body-code"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}

			rec := deleteMeAt(t, codedEcho(testAuthHandlers(svc)), "/auth/me"+query, bearerToken(t, uuid.New()), tt.body)

			assertHTTPStatus(t, rec, http.StatusNoContent)
			if len(svc.deleteProofs) != 1 || svc.deleteProofs[0] != tt.want {
				t.Errorf("proofs = %+v, want one %+v", svc.deleteProofs, tt.want)
			}
		})
	}
}

// A half proof in the body is refused even when the query string completes it.
func TestDeleteMe_theQueryStringDoesNotCompleteAHalfProof(t *testing.T) {
	svc := &fakeAuthService{}
	target := "/auth/me?apple_authorization_code=" + secretCode

	rec := deleteMeAt(t, codedEcho(testAuthHandlers(svc)), target, bearerToken(t, uuid.New()), `{"apple_id_token":"body-token"}`)

	assertCodedError(t, rec, 400, "bad_request", "apple_id_token and apple_authorization_code must be sent together")
	if len(svc.deleteCalls) != 0 {
		t.Error("service must not be called")
	}
}

func TestDeleteMe_theProofFieldsGoTogether(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{"token without code", `{"apple_id_token":"t"}`, "apple_id_token and apple_authorization_code must be sent together"},
		{"code without token", `{"apple_authorization_code":"c"}`, "apple_id_token and apple_authorization_code must be sent together"},
		{"token with an empty code", `{"apple_id_token":"t","apple_authorization_code":""}`, "apple_id_token and apple_authorization_code must be sent together"},
		{"redirect address alone", `{"apple_redirect_uri":"` + redirectURI + `"}`, "apple_redirect_uri needs apple_id_token and apple_authorization_code"},
		{"redirect address with the token only", `{"apple_id_token":"t","apple_redirect_uri":"` + redirectURI + `"}`, "apple_id_token and apple_authorization_code must be sent together"},
		{"redirect address with the code only", `{"apple_authorization_code":"c","apple_redirect_uri":"` + redirectURI + `"}`, "apple_id_token and apple_authorization_code must be sent together"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}

			rec := deleteMe(t, codedEcho(testAuthHandlers(svc)), bearerToken(t, uuid.New()), tt.body)

			assertCodedError(t, rec, 400, "bad_request", tt.wantMessage)
			if len(svc.deleteCalls) != 0 {
				t.Error("service must not be called")
			}
		})
	}
}

func TestDeleteMe_theRedirectAddressMustBeAnAbsoluteHTTPSURL(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", maxAppleRedirectURILen-len("https://example.com/"))
	tests := []struct {
		name  string
		uri   string
		valid bool
	}{
		{"the callback of the API", redirectURI, true},
		{"a bare host", "https://example.com", true},
		{"a port and a query", "https://example.com:8443/cb?x=1", true},
		{"an upper case scheme", "HTTPS://EXAMPLE.COM/cb", true},
		{"exactly the longest allowed", long, true},
		{"one character more", long + "a", false},
		{"http", "http://example.com/cb", false},
		{"another scheme", "ftp://example.com/cb", false},
		{"a custom scheme", "com.mentorix.mobile://callback", false},
		{"javascript", "javascript:alert(1)", false},
		{"no scheme", "example.com/cb", false},
		{"scheme-relative", "//example.com/cb", false},
		{"a path only", "/auth/apple/callback", false},
		{"no host", "https://", false},
		{"no host, a path", "https:///cb", false},
		{"a fragment", "https://example.com/cb#frag", false},
		{"an empty fragment", "https://example.com/cb#", false},
		{"a space inside the host", "https://exa mple.com/cb", false},
		{"a control character", "https://example.com/cb\n", false},
		{"only spaces", "   ", false},
		{"not a url", "::::", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			body, err := json.Marshal(map[string]string{"apple_id_token": "t", "apple_authorization_code": "c", "apple_redirect_uri": tt.uri})
			if err != nil {
				t.Fatal(err)
			}

			rec := deleteMe(t, codedEcho(testAuthHandlers(svc)), bearerToken(t, uuid.New()), string(body))

			if tt.valid {
				assertHTTPStatus(t, rec, http.StatusNoContent)
				if len(svc.deleteProofs) != 1 || svc.deleteProofs[0].RedirectURI != tt.uri {
					t.Errorf("proofs = %+v, want the redirect address %q passed on", svc.deleteProofs, tt.uri)
				}
				return
			}
			assertCodedError(t, rec, 400, "bad_request",
				fmt.Sprintf("apple_redirect_uri must be an absolute https URL without a fragment, at most %d characters", maxAppleRedirectURILen))
			if len(svc.deleteCalls) != 0 {
				t.Error("service must not be called")
			}
		})
	}
}

// The route carries its own body limit, like /auth/apple/callback.
func TestDeleteMe_bodyOverTheLimitIsRefused(t *testing.T) {
	big := `{"apple_id_token":"` + strings.Repeat("a", 2*16000) + `","apple_authorization_code":"c"}`
	tests := []struct {
		name string
		body io.Reader
	}{
		{"known length", strings.NewReader(big)},
		{"unknown length", io.NopCloser(strings.NewReader(big))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			req := httptest.NewRequest(http.MethodDelete, "/auth/me", tt.body)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set(echo.HeaderAuthorization, bearerToken(t, uuid.New()))
			rec := httptest.NewRecorder()

			codedEcho(testAuthHandlers(svc)).ServeHTTP(rec, req)

			assertCodedError(t, rec, 413, "payload_too_large", "Request Entity Too Large")
			if len(svc.deleteCalls) != 0 {
				t.Error("service must not be called")
			}
		})
	}
}

// The limit is exact whatever the framing: 16000 bytes pass and one more does not. The
// number is written out, not taken from the route's constant, so a changed limit fails here;
// the contract says 16 KB.
func TestDeleteMe_bodyLimitBoundary(t *testing.T) {
	const prefix, suffix = `{"apple_id_token":"`, `","apple_authorization_code":"c"}`
	bodyOf := func(size int) string {
		return prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
	}
	tests := []struct {
		name   string
		size   int
		framed func(string) io.Reader
		want   int
	}{
		{"at the limit, known length", 16000, func(b string) io.Reader { return strings.NewReader(b) }, http.StatusNoContent},
		{"over the limit, known length", 16000 + 1, func(b string) io.Reader { return strings.NewReader(b) }, http.StatusRequestEntityTooLarge},
		{"at the limit, unknown length", 16000, func(b string) io.Reader { return io.NopCloser(strings.NewReader(b)) }, http.StatusNoContent},
		{"over the limit, unknown length", 16000 + 1, func(b string) io.Reader { return io.NopCloser(strings.NewReader(b)) }, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			req := httptest.NewRequest(http.MethodDelete, "/auth/me", tt.framed(bodyOf(tt.size)))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set(echo.HeaderAuthorization, bearerToken(t, uuid.New()))
			rec := httptest.NewRecorder()

			codedEcho(testAuthHandlers(svc)).ServeHTTP(rec, req)

			assertHTTPStatus(t, rec, tt.want)
		})
	}
}

func TestDeleteMe_aBodyUnderTheLimitPasses(t *testing.T) {
	svc := &fakeAuthService{}
	body := deleteBody(strings.Repeat("a", 8000), "c", redirectURI)

	rec := deleteMe(t, codedEcho(testAuthHandlers(svc)), bearerToken(t, uuid.New()), body)

	assertHTTPStatus(t, rec, http.StatusNoContent)
}

// Neither the code nor the token reaches the request log, on any answer: the log shows
// the cause of a failure, and the cause never holds the request values.
func TestDeleteMe_noRequestValueReachesTheLog(t *testing.T) {
	valid := deleteBody(secretIDToken, secretCode, redirectURI)
	tests := []struct {
		name string
		svc  *fakeAuthService
		body string
	}{
		{"success", &fakeAuthService{}, valid},
		{"invalid id token", &fakeAuthService{deleteErr: fmt.Errorf("%w: expired", ErrInvalidIDToken)}, valid},
		{"invalid code", &fakeAuthService{deleteErr: fmt.Errorf("%w: apple answered 400 invalid_grant", ErrInvalidAppleAuthorizationCode)}, valid},
		{"revocation failed", &fakeAuthService{deleteErr: fmt.Errorf("%w: apple answered 502", ErrAppleRevocationFailed)}, valid},
		{"mismatch", &fakeAuthService{deleteErr: ErrAppleAccountMismatch}, valid},
		{"required", &fakeAuthService{deleteErr: ErrAppleAuthorizationRequired}, valid},
		{"internal error", &fakeAuthService{deleteErr: errors.New("db down")}, valid},
		{"invalid json", &fakeAuthService{}, `{"apple_id_token":"` + secretIDToken + `","apple_authorization_code":"` + secretCode},
		{"half proof", &fakeAuthService{}, `{"apple_id_token":"` + secretIDToken + `"}`},
		{"bad redirect address", &fakeAuthService{}, deleteBody(secretIDToken, secretCode, "http://"+secretCode+".example/cb")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			e := codedEcho(testAuthHandlers(tt.svc), httpx.RequestLog(slog.New(slog.NewTextHandler(&logs, nil))))

			rec := deleteMe(t, e, bearerToken(t, uuid.New()), tt.body)

			if logs.Len() == 0 {
				t.Fatal("nothing was logged, the check proves nothing")
			}
			for _, secret := range []string{secretIDToken, secretCode, redirectURI} {
				if strings.Contains(logs.String(), secret) || strings.Contains(rec.Body.String(), secret) {
					t.Errorf("%q reached the log or the body\nlog: %s\nbody: %s", secret, logs.String(), rec.Body)
				}
			}
		})
	}
}
