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
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/subscription"
)

// activationRecorder holds what the fake service saw of ActivateTrainer calls.
type activationRecorder struct {
	users []uuid.UUID
	terms []string
}

func (f *fakeAuthService) ActivateTrainer(_ context.Context, userID uuid.UUID, termsVersion string) (UserProfile, error) {
	f.activate.users = append(f.activate.users, userID)
	f.activate.terms = append(f.activate.terms, termsVersion)
	if f.activateErr != nil {
		return UserProfile{}, f.activateErr
	}
	return f.profile, nil
}

// postActivateTrainer sends POST /auth/me/trainer with body as JSON, or without a body when it is empty.
func postActivateTrainer(t *testing.T, e *echo.Echo, authorization, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/me/trainer", strings.NewReader(body))
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

func TestHandlers_Mount_registersActivateTrainerRoute(t *testing.T) {
	e := echo.New()
	testAuthHandlers(&fakeAuthService{}).Mount(e)
	for _, r := range e.Routes() {
		if r.Method == http.MethodPost && r.Path == "/auth/me/trainer" {
			return
		}
	}
	t.Fatal("missing route POST /auth/me/trainer")
}

func TestActivateTrainer_success(t *testing.T) {
	userID := uuid.New()
	createdAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	svc := &fakeAuthService{profile: UserProfile{
		Email:     "client@test.com",
		Name:      "Anna",
		CreatedAt: createdAt,
		Roles:     []string{RoleClient, RoleTrainer},
	}}
	h := testAuthHandlers(svc)
	h.subs = &fakeSubs{sub: &subscription.Subscription{Plan: subscription.PlanFree}}
	e := codedEcho(h)

	rec := postActivateTrainer(t, e, bearerToken(t, userID), `{"terms_version":"2026-10-01"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.UserID != userID.String() || resp.Email != "client@test.com" || resp.Name != "Anna" || !resp.CreatedAt.Equal(createdAt) {
		t.Errorf("resp = %+v", resp)
	}
	if len(resp.Roles) != 2 || resp.Roles[0] != RoleClient || resp.Roles[1] != RoleTrainer {
		t.Errorf("roles = %v, want [client trainer]", resp.Roles)
	}
	if resp.Subscription == nil || resp.Subscription.Plan != subscription.PlanFree {
		t.Errorf("subscription = %+v, want the free plan", resp.Subscription)
	}
	if len(svc.activate.users) != 1 || svc.activate.users[0] != userID || svc.activate.terms[0] != "2026-10-01" {
		t.Errorf("service called with %v, %q, want [%v], [2026-10-01]", svc.activate.users, svc.activate.terms, userID)
	}
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
}

// A repeat is the same call: the service decides whether anything changes, the answer is 200 either way.
func TestActivateTrainer_repeatAnswersOKAgain(t *testing.T) {
	svc := &fakeAuthService{profile: UserProfile{Roles: []string{RoleClient, RoleTrainer}}}
	e := codedEcho(testAuthHandlers(svc))
	token := bearerToken(t, uuid.New())

	for range 2 {
		assertHTTPStatus(t, postActivateTrainer(t, e, token, `{"terms_version":"2026-10-01"}`), http.StatusOK)
	}

	if len(svc.activate.users) != 2 {
		t.Errorf("service calls = %d, want 2", len(svc.activate.users))
	}
}

func TestActivateTrainer_trimsTheVersion(t *testing.T) {
	svc := &fakeAuthService{}
	e := codedEcho(testAuthHandlers(svc))

	rec := postActivateTrainer(t, e, bearerToken(t, uuid.New()), `{"terms_version":"  2026-10-01\t"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if len(svc.activate.terms) != 1 || svc.activate.terms[0] != "2026-10-01" {
		t.Errorf("service got %q, want the trimmed 2026-10-01", svc.activate.terms)
	}
}

func TestActivateTrainer_badBodyNeverReachesTheService(t *testing.T) {
	const dateMessage = "terms_version must be a date in YYYY-MM-DD form"
	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{"no body", "", dateMessage},
		{"empty object", `{}`, dateMessage},
		{"null", `{"terms_version":null}`, dateMessage},
		{"empty string", `{"terms_version":""}`, dateMessage},
		{"blank string", `{"terms_version":"   "}`, dateMessage},
		{"month 13", `{"terms_version":"2026-13-01"}`, dateMessage},
		{"day without a leading zero", `{"terms_version":"2026-10-1"}`, dateMessage},
		{"month without a leading zero", `{"terms_version":"2026-1-01"}`, dateMessage},
		{"day 30 of February", `{"terms_version":"2026-02-30"}`, dateMessage},
		{"29 February of a common year", `{"terms_version":"2026-02-29"}`, dateMessage},
		{"no dashes", `{"terms_version":"20261001"}`, dateMessage},
		{"slashes", `{"terms_version":"2026/10/01"}`, dateMessage},
		{"a timestamp", `{"terms_version":"2026-10-01T00:00:00Z"}`, dateMessage},
		{"a date with text after it", `{"terms_version":"2026-10-01 v2"}`, dateMessage},
		{"inner space", `{"terms_version":"2026-10- 01"}`, dateMessage},
		{"letters", `{"terms_version":"latest"}`, dateMessage},
		{"non-ASCII digits", `{"terms_version":"٢٠٢٦-١٠-٠١"}`, dateMessage},
		{"not JSON", `not-json`, httpx.MsgInvalidJSON},
		{"a number", `{"terms_version":20261001}`, httpx.MsgInvalidJSON},
		{"a JSON array", `["2026-10-01"]`, httpx.MsgInvalidJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			e := codedEcho(testAuthHandlers(svc))

			rec := postActivateTrainer(t, e, bearerToken(t, uuid.New()), tt.body)

			assertCodedError(t, rec, http.StatusBadRequest, "bad_request", tt.wantMessage)
			if len(svc.activate.users) != 0 {
				t.Errorf("the service was called for a bad body: %v", svc.activate.terms)
			}
		})
	}
}

func TestActivateTrainer_acceptsLeapDay(t *testing.T) {
	svc := &fakeAuthService{}
	e := codedEcho(testAuthHandlers(svc))

	rec := postActivateTrainer(t, e, bearerToken(t, uuid.New()), `{"terms_version":"2028-02-29"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
}

func TestActivateTrainer_requiresToken(t *testing.T) {
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
			e := codedEcho(testAuthHandlers(svc))

			rec := postActivateTrainer(t, e, tt.authorization, `{"terms_version":"2026-10-01"}`)

			assertCodedError(t, rec, http.StatusUnauthorized, tt.wantCode, tt.wantMessage)
			if len(svc.activate.users) != 0 {
				t.Error("the service was called without a valid token")
			}
		})
	}
}

func TestActivateTrainer_bodyLimit(t *testing.T) {
	svc := &fakeAuthService{}
	e := codedEcho(testAuthHandlers(svc))
	body := `{"terms_version":"2026-10-01","pad":"` + strings.Repeat("a", 16000) + `"}`

	rec := postActivateTrainer(t, e, bearerToken(t, uuid.New()), body)

	assertCodedError(t, rec, http.StatusRequestEntityTooLarge, "payload_too_large", "Request Entity Too Large")
	if len(svc.activate.users) != 0 {
		t.Error("the service was called for an oversized body")
	}
}

func TestActivateTrainer_serviceErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"admin", ErrAdminCannotActivate, 403, "admin_cannot_activate", ErrAdminCannotActivate.Error()},
		{"admin, wrapped", fmt.Errorf("activate trainer: %w", ErrAdminCannotActivate), 403, "admin_cannot_activate", ErrAdminCannotActivate.Error()},
		{"account gone", pgx.ErrNoRows, 404, "user_not_found", httpx.MsgUserNotFound},
		{"account gone, wrapped", fmt.Errorf("load profile: %w", pgx.ErrNoRows), 404, "user_not_found", httpx.MsgUserNotFound},
		{"database failure", errors.New("db down"), 500, "internal_error", "trainer activation failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := codedEcho(testAuthHandlers(&fakeAuthService{activateErr: tt.err}))

			rec := postActivateTrainer(t, e, bearerToken(t, uuid.New()), `{"terms_version":"2026-10-01"}`)

			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

// The cause of a 500 is in the log and never in the body.
func TestActivateTrainer_internalErrorKeepsTheCauseOutOfTheBody(t *testing.T) {
	var logs bytes.Buffer
	cause := errors.New("connection to 10.0.0.5 refused")
	e := codedEcho(testAuthHandlers(&fakeAuthService{activateErr: cause}),
		httpx.RequestLog(slog.New(slog.NewTextHandler(&logs, nil))))

	rec := postActivateTrainer(t, e, bearerToken(t, uuid.New()), `{"terms_version":"2026-10-01"}`)

	assertCodedError(t, rec, http.StatusInternalServerError, "internal_error", "trainer activation failed")
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("body leaks the cause: %s", rec.Body)
	}
	if !strings.Contains(logs.String(), "10.0.0.5") {
		t.Errorf("log %q does not carry the cause", logs.String())
	}
}

func TestActivateTrainer_subscriptionErrorIsInternal(t *testing.T) {
	h := testAuthHandlers(&fakeAuthService{})
	h.subs = &fakeSubs{err: errors.New("db down")}

	rec := postActivateTrainer(t, codedEcho(h), bearerToken(t, uuid.New()), `{"terms_version":"2026-10-01"}`)

	assertCodedError(t, rec, http.StatusInternalServerError, "internal_error", httpx.MsgInternal)
}

func TestActivateTrainer_missingUserInContext(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/me/trainer", h.ActivateTrainer)

	rec := postActivateTrainer(t, e, "", `{"terms_version":"2026-10-01"}`)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
}

// A body of unknown length is cut by the limit while it is read, not up front. The decoder
// reads in growing chunks and checks the error only after it scanned them, so a body
// that barely passes the limit can still be read whole; a much larger one cannot.
func TestActivateTrainer_streamedBodyOverTheLimit(t *testing.T) {
	svc := &fakeAuthService{}
	e := codedEcho(testAuthHandlers(svc))
	body := `{"terms_version":"` + strings.Repeat("a", 100000) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/me/trainer", io.NopCloser(strings.NewReader(body)))
	req.ContentLength = -1
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, bearerToken(t, uuid.New()))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assertCodedError(t, rec, http.StatusRequestEntityTooLarge, "payload_too_large", "Request Entity Too Large")
	if len(svc.activate.users) != 0 {
		t.Error("the service was called for an oversized body")
	}
}
