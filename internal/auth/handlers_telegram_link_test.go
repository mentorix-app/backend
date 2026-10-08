package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	httpx "mentorix-backend/internal/http"
)

// linkEcho mounts the real routes so the JWT middleware is part of the test.
func linkEcho(h *Handlers) *echo.Echo {
	e := echo.New()
	h.Mount(e)
	return e
}

func postLink(t *testing.T, e *echo.Echo, userID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/telegram/link", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if userID != uuid.Nil {
		token, _, err := signAccessToken(userID, []byte(testJWTSecret), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestHandlers_Mount_registersTelegramLinkRoute(t *testing.T) {
	e := linkEcho(testAuthHandlers(&fakeAuthService{}))
	for _, r := range e.Routes() {
		if r.Method == http.MethodPost && r.Path == "/auth/telegram/link" {
			return
		}
	}
	t.Fatal("missing route POST /auth/telegram/link")
}

func TestTelegramLink_success(t *testing.T) {
	appUser, remaining := uuid.New(), uuid.New()
	svc := &fakeAuthService{linkIssued: IssuedAuth{UserID: remaining, Email: "old@example.com", RefreshToken: "body-refresh"}}
	e := linkEcho(testAuthHandlers(svc))

	rec := postLink(t, e, appUser, `{"code":"ABCD2345"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if svc.linkUserID != appUser || svc.linkCode != "ABCD2345" {
		t.Errorf("service called with %v, %q", svc.linkUserID, svc.linkCode)
	}
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.RefreshToken != "body-refresh" || resp.AccessToken == "" || resp.UserID != remaining.String() || resp.Email != "old@example.com" {
		t.Errorf("response = %+v", resp)
	}
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
}

func TestTelegramLink_errors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"invalid json", `not-json`, nil, http.StatusBadRequest, httpx.MsgInvalidJSON},
		{"invalid code", `{"code":"x"}`, ErrInvalidLinkCode, http.StatusBadRequest, "invalid or expired code"},
		{"missing code", `{}`, ErrInvalidLinkCode, http.StatusBadRequest, "invalid or expired code"},
		{"app user gone", `{"code":"x"}`, pgx.ErrNoRows, http.StatusNotFound, httpx.MsgUserNotFound},
		{"conflict", `{"code":"x"}`, ErrTelegramLinkConflict, http.StatusConflict, "telegram is linked to an account that cannot be merged"},
		{"already linked", `{"code":"x"}`, ErrTelegramAlreadyLinked, http.StatusConflict, "account already has another telegram"},
		{"codes unavailable", `{"code":"x"}`, ErrLinkCodesUnavailable, http.StatusServiceUnavailable, "telegram linking is unavailable"},
		{"internal", `{"code":"x"}`, errors.New("db down"), http.StatusInternalServerError, "telegram link failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := linkEcho(testAuthHandlers(&fakeAuthService{linkErr: tt.err}))
			rec := postLink(t, e, uuid.New(), tt.body)
			assertHTTPStatus(t, rec, tt.wantStatus)
			if !strings.Contains(rec.Body.String(), tt.wantMsg) {
				t.Errorf("body = %s, want message %q", rec.Body.String(), tt.wantMsg)
			}
			if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
				t.Errorf("Set-Cookie = %v, want none", got)
			}
		})
	}
}

func TestTelegramLink_requiresToken(t *testing.T) {
	svc := &fakeAuthService{}
	e := linkEcho(testAuthHandlers(svc))

	rec := postLink(t, e, uuid.Nil, `{"code":"ABCD2345"}`)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
	if svc.linkCode != "" {
		t.Error("service must not be called without a token")
	}
}

func TestTelegramLink_causeReachesRequestLog(t *testing.T) {
	var buf bytes.Buffer
	e := echo.New()
	e.Use(httpx.RequestLog(slog.New(slog.NewTextHandler(&buf, nil))))
	testAuthHandlers(&fakeAuthService{linkErr: errors.New("db down")}).Mount(e)

	rec := postLink(t, e, uuid.New(), `{"code":"SECRET23"}`)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), "db down") {
		t.Errorf("body leaks the cause: %s", rec.Body.String())
	}
	if !strings.Contains(buf.String(), "db down") {
		t.Errorf("log does not carry the cause: %s", buf.String())
	}
	if strings.Contains(buf.String(), "SECRET23") {
		t.Errorf("log carries the code: %s", buf.String())
	}
}

func TestTelegramLink_rateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	e := linkEcho(h)
	user := uuid.New()

	assertHTTPStatus(t, postLink(t, e, user, `{"code":"ABCD2345"}`), http.StatusOK)
	assertHTTPStatus(t, postLink(t, e, user, `{"code":"ABCD2345"}`), http.StatusTooManyRequests)
}

func TestMe_reportsTelegramLinked(t *testing.T) {
	for _, linked := range []bool{true, false} {
		h := testAuthHandlers(&fakeAuthService{profile: UserProfile{Email: "a@b.com", Roles: []string{RoleClient}, TelegramLinked: linked}})
		e := echo.New()
		e.GET("/auth/me", h.Me, func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				c.Set(ContextUserIDKey, uuid.New())
				return next(c)
			}
		})
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assertHTTPStatus(t, rec, http.StatusOK)

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		want := "false"
		if linked {
			want = "true"
		}
		if got := string(raw["telegram_linked"]); got != want {
			t.Errorf("telegram_linked = %q, want %q (body %s)", got, want, rec.Body.String())
		}
	}
}

func TestUpdateMe_reportsTelegramLinked(t *testing.T) {
	h := testAuthHandlers(&fakeAuthService{profile: UserProfile{TelegramLinked: true}})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader(`{"name":"X"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(ContextUserIDKey, uuid.New())

	if err := h.UpdateMe(c); err != nil {
		t.Fatalf("UpdateMe: %v", err)
	}
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.TelegramLinked {
		t.Error("telegram_linked = false, want true")
	}
}
