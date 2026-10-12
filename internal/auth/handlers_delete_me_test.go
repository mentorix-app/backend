package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func (f *fakeAuthService) DeleteAccount(_ context.Context, userID uuid.UUID) error {
	f.deleteCalls = append(f.deleteCalls, userID)
	return f.deleteErr
}

func deleteMe(t *testing.T, e *echo.Echo, authorization, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/auth/me", strings.NewReader(body))
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

// The endpoint takes no request body, so whatever arrives with the call is not read.
func TestDeleteMe_ignoresTheRequestBody(t *testing.T) {
	svc := &fakeAuthService{}
	e := codedEcho(testAuthHandlers(svc))

	rec := deleteMe(t, e, bearerToken(t, uuid.New()), "not-json")

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if len(svc.deleteCalls) != 1 {
		t.Errorf("service calls = %d, want 1", len(svc.deleteCalls))
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
