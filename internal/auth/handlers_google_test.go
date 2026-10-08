package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	httpx "mentorix-backend/internal/http"
)

func postJSON(e *echo.Echo, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestHandlers_Mount_registersGoogleRoute(t *testing.T) {
	e := echo.New()
	testAuthHandlers(&fakeAuthService{}).Mount(e)
	for _, r := range e.Routes() {
		if r.Method == http.MethodPost && r.Path == "/auth/google" {
			return
		}
	}
	t.Fatal("missing route POST /auth/google")
}

func TestGoogle_success(t *testing.T) {
	userID := uuid.New()
	svc := &fakeAuthService{googleIssued: IssuedAuth{UserID: userID, Email: "g@example.com", RefreshToken: "body-refresh"}}
	e := echo.New()
	e.POST("/auth/google", testAuthHandlers(svc).Google)

	rec := postJSON(e, "/auth/google", `{"id_token":"raw-token"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if svc.googleToken != "raw-token" {
		t.Errorf("service token = %q", svc.googleToken)
	}
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.RefreshToken != "body-refresh" || resp.AccessToken == "" || resp.UserID != userID.String() || resp.Email != "g@example.com" {
		t.Errorf("response = %+v", resp)
	}
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
}

func TestGoogle_errors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		svc        *fakeAuthService
		wantStatus int
	}{
		{"invalid json", `not-json`, &fakeAuthService{}, http.StatusBadRequest},
		{"empty id_token", `{"id_token":""}`, &fakeAuthService{}, http.StatusBadRequest},
		{"missing id_token", `{}`, &fakeAuthService{}, http.StatusBadRequest},
		{"invalid token", `{"id_token":"x"}`, &fakeAuthService{googleErr: ErrInvalidIDToken}, http.StatusUnauthorized},
		{"not configured", `{"id_token":"x"}`, &fakeAuthService{googleErr: ErrProviderNotConfigured}, http.StatusServiceUnavailable},
		{"internal", `{"id_token":"x"}`, &fakeAuthService{googleErr: errors.New("db down")}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			e.POST("/auth/google", testAuthHandlers(tt.svc).Google)
			rec := postJSON(e, "/auth/google", tt.body)
			assertHTTPStatus(t, rec, tt.wantStatus)
			if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
				t.Errorf("Set-Cookie = %v, want none", got)
			}
		})
	}
}

func TestGoogle_causeReachesRequestLog(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"invalid token", fmt.Errorf("%w: token is expired", ErrInvalidIDToken), http.StatusUnauthorized, ErrInvalidIDToken.Error()},
		{"internal", errors.New("db down"), http.StatusInternalServerError, "google sign-in failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			e := echo.New()
			e.Use(httpx.RequestLog(slog.New(slog.NewTextHandler(&buf, nil))))
			e.POST("/auth/google", testAuthHandlers(&fakeAuthService{googleErr: tt.err}).Google)

			rec := postJSON(e, "/auth/google", `{"id_token":"secret-raw-token"}`)

			assertHTTPStatus(t, rec, tt.wantStatus)
			if !strings.Contains(rec.Body.String(), tt.wantMsg) || strings.Contains(rec.Body.String(), tt.err.Error()) && tt.err.Error() != tt.wantMsg {
				t.Errorf("body = %s, want constant message %q", rec.Body.String(), tt.wantMsg)
			}
			if !strings.Contains(buf.String(), tt.err.Error()) {
				t.Errorf("log does not carry the cause: %s", buf.String())
			}
			if strings.Contains(buf.String(), "secret-raw-token") {
				t.Errorf("log carries the raw token: %s", buf.String())
			}
		})
	}
}

func TestGoogle_notConfiguredMessage(t *testing.T) {
	e := echo.New()
	e.POST("/auth/google", testAuthHandlers(&fakeAuthService{googleErr: ErrProviderNotConfigured}).Google)
	rec := postJSON(e, "/auth/google", `{"id_token":"x"}`)
	if !strings.Contains(rec.Body.String(), "google sign-in is not configured") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestGoogle_rateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	e := echo.New()
	e.POST("/auth/google", h.Google)

	for range 2 {
		postJSON(e, "/auth/google", `{"id_token":"x"}`)
	}
	rec := postJSON(e, "/auth/google", `{"id_token":"x"}`)
	assertHTTPStatus(t, rec, http.StatusTooManyRequests)
}

func TestCookieFlowsOmitRefreshTokenFromBody(t *testing.T) {
	e := echo.New()
	h := testAuthHandlers(&fakeAuthService{})
	e.POST("/auth/register", h.Register)
	e.POST("/auth/login", h.Login)
	e.POST("/auth/refresh", h.Refresh)

	reg := postJSON(e, "/auth/register", `{"email":"user@example.com","password":"password123"}`)
	login := postJSON(e, "/auth/login", `{"email":"user@example.com","password":"password123"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-token"})
	refresh := httptest.NewRecorder()
	e.ServeHTTP(refresh, req)

	for name, rec := range map[string]*httptest.ResponseRecorder{"register": reg, "login": login, "refresh": refresh} {
		if strings.Contains(rec.Body.String(), "refresh_token") {
			t.Errorf("%s body carries refresh_token: %s", name, rec.Body.String())
		}
	}
}

func TestRefresh_bodyMode(t *testing.T) {
	svc := &fakeAuthService{refreshIssued: IssuedAuth{RefreshToken: "rotated"}}
	e := echo.New()
	e.POST("/auth/refresh", testAuthHandlers(svc).Refresh)

	rec := postJSON(e, "/auth/refresh", `{"refresh_token":"body-token"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if svc.refreshToken != "body-token" {
		t.Errorf("service token = %q", svc.refreshToken)
	}
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.RefreshToken != "rotated" {
		t.Errorf("refresh_token = %q", resp.RefreshToken)
	}
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
}

func TestRefresh_bodyModeWinsOverCookie(t *testing.T) {
	svc := &fakeAuthService{}
	e := echo.New()
	e.POST("/auth/refresh", testAuthHandlers(svc).Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refresh_token":"body-token"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusOK)
	if svc.refreshToken != "body-token" {
		t.Errorf("service token = %q, want body token", svc.refreshToken)
	}
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
}

func TestRefresh_bodyModeInvalidTokenKeepsCookies(t *testing.T) {
	e := echo.New()
	e.POST("/auth/refresh", testAuthHandlers(&fakeAuthService{refreshErr: ErrInvalidRefresh}).Refresh)

	rec := postJSON(e, "/auth/refresh", `{"refresh_token":"stale"}`)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
}

func TestRefresh_emptyBodyFallsBackToCookie(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
	}{
		{"no body", "", ""},
		{"json content type, empty body", "", echo.MIMEApplicationJSON},
		{"empty object", `{}`, echo.MIMEApplicationJSON},
		{"empty refresh_token", `{"refresh_token":""}`, echo.MIMEApplicationJSON},
		{"body without content type", `{"refresh_token":"ignored"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuthService{}
			e := echo.New()
			e.POST("/auth/refresh", testAuthHandlers(svc).Refresh)

			req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set(echo.HeaderContentType, tt.contentType)
			}
			req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-token"})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assertHTTPStatus(t, rec, http.StatusOK)
			if svc.refreshToken != "cookie-token" {
				t.Errorf("service token = %q, want cookie token", svc.refreshToken)
			}
			if strings.Contains(rec.Body.String(), "refresh_token") {
				t.Errorf("body carries refresh_token: %s", rec.Body.String())
			}
			cookies := rec.Result().Cookies()
			if len(cookies) == 0 || cookies[0].Value == "" {
				t.Error("expected refresh cookie")
			}
		})
	}
}

func TestRefresh_malformedJSONBodyUsesCookie(t *testing.T) {
	svc := &fakeAuthService{}
	e := echo.New()
	e.POST("/auth/refresh", testAuthHandlers(svc).Refresh)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refresh_token":`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusOK)
	if svc.refreshToken != "cookie-token" {
		t.Errorf("service token = %q, want cookie token", svc.refreshToken)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Value == "" {
		t.Error("expected refresh cookie")
	}
}

func TestRefresh_malformedJSONBodyWithoutCookie(t *testing.T) {
	e := echo.New()
	e.POST("/auth/refresh", testAuthHandlers(&fakeAuthService{}).Refresh)

	rec := postJSON(e, "/auth/refresh", `{"refresh_token":`)

	assertHTTPStatus(t, rec, http.StatusUnauthorized)
	if !strings.Contains(rec.Body.String(), "missing refresh token") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestLogout_bodyMode(t *testing.T) {
	svc := &fakeAuthService{}
	e := echo.New()
	e.POST("/auth/logout", testAuthHandlers(svc).Logout)

	rec := postJSON(e, "/auth/logout", `{"refresh_token":"body-token"}`)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if svc.logoutToken != "body-token" {
		t.Errorf("service token = %q", svc.logoutToken)
	}
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
}

func TestLogout_bodyModeRevokeError(t *testing.T) {
	e := echo.New()
	e.POST("/auth/logout", testAuthHandlers(&fakeAuthService{logoutErr: errors.New("db down")}).Logout)

	rec := postJSON(e, "/auth/logout", `{"refresh_token":"body-token"}`)

	assertHTTPStatus(t, rec, http.StatusInternalServerError)
}

func TestLogout_emptyBodyUsesCookie(t *testing.T) {
	svc := &fakeAuthService{}
	e := echo.New()
	e.POST("/auth/logout", testAuthHandlers(svc).Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	if svc.logoutToken != "cookie-token" {
		t.Errorf("service token = %q, want cookie token", svc.logoutToken)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge != -1 {
		t.Errorf("cookies = %v, want a clearing cookie", cookies)
	}
}

func TestLogout_malformedJSONBodyClearsCookie(t *testing.T) {
	e := echo.New()
	e.POST("/auth/logout", testAuthHandlers(&fakeAuthService{}).Logout)

	rec := postJSON(e, "/auth/logout", `{"refresh_token":`)

	assertHTTPStatus(t, rec, http.StatusNoContent)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge != -1 {
		t.Errorf("cookies = %v, want a clearing cookie", cookies)
	}
}
