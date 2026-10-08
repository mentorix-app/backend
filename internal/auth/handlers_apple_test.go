package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	httpx "mentorix-backend/internal/http"
)

func TestHandlers_Mount_registersAppleRoute(t *testing.T) {
	e := echo.New()
	testAuthHandlers(&fakeAuthService{}).Mount(e)
	for _, r := range e.Routes() {
		if r.Method == http.MethodPost && r.Path == "/auth/apple" {
			return
		}
	}
	t.Fatal("missing route POST /auth/apple")
}

func TestApple_success(t *testing.T) {
	userID := uuid.New()
	svc := &fakeAuthService{appleIssued: IssuedAuth{UserID: userID, Email: "a@example.com", RefreshToken: "body-refresh"}}
	e := echo.New()
	e.POST("/auth/apple", testAuthHandlers(svc).Apple)

	rec := postJSON(e, "/auth/apple", `{"id_token":"raw-token","name":"Apple Person"}`)

	assertHTTPStatus(t, rec, http.StatusOK)
	if svc.appleToken != "raw-token" || svc.appleName != "Apple Person" {
		t.Errorf("service got token %q, name %q", svc.appleToken, svc.appleName)
	}
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.RefreshToken != "body-refresh" || resp.AccessToken == "" || resp.UserID != userID.String() {
		t.Errorf("response = %+v", resp)
	}
	if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %v, want none", got)
	}
}

func TestApple_nameIsOptionalAndNeverRejected(t *testing.T) {
	long := strings.Repeat("я", 500)
	for _, body := range []string{`{"id_token":"x"}`, `{"id_token":"x","name":"` + long + `"}`} {
		e := echo.New()
		e.POST("/auth/apple", testAuthHandlers(&fakeAuthService{}).Apple)
		assertHTTPStatus(t, postJSON(e, "/auth/apple", body), http.StatusOK)
	}
}

func TestApple_errors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		svc        *fakeAuthService
		wantStatus int
	}{
		{"invalid json", `not-json`, &fakeAuthService{}, http.StatusBadRequest},
		{"empty id_token", `{"id_token":""}`, &fakeAuthService{}, http.StatusBadRequest},
		{"missing id_token", `{}`, &fakeAuthService{}, http.StatusBadRequest},
		{"invalid token", `{"id_token":"x"}`, &fakeAuthService{appleErr: ErrInvalidIDToken}, http.StatusUnauthorized},
		{"not configured", `{"id_token":"x"}`, &fakeAuthService{appleErr: ErrProviderNotConfigured}, http.StatusServiceUnavailable},
		{"internal", `{"id_token":"x"}`, &fakeAuthService{appleErr: errors.New("db down")}, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			e.POST("/auth/apple", testAuthHandlers(tt.svc).Apple)
			rec := postJSON(e, "/auth/apple", tt.body)
			assertHTTPStatus(t, rec, tt.wantStatus)
			if got := rec.Result().Header.Values("Set-Cookie"); len(got) != 0 {
				t.Errorf("Set-Cookie = %v, want none", got)
			}
		})
	}
}

func TestApple_notConfiguredMessage(t *testing.T) {
	e := echo.New()
	e.POST("/auth/apple", testAuthHandlers(&fakeAuthService{appleErr: ErrProviderNotConfigured}).Apple)
	rec := postJSON(e, "/auth/apple", `{"id_token":"x"}`)
	if !strings.Contains(rec.Body.String(), "apple sign-in is not configured") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestApple_rateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	h := testAuthHandlers(&fakeAuthService{})
	h.limiter = NewRateLimiter(rdb, 1, time.Minute, 5, time.Minute)
	e := echo.New()
	e.POST("/auth/apple", h.Apple)

	for range 2 {
		postJSON(e, "/auth/apple", `{"id_token":"x"}`)
	}
	assertHTTPStatus(t, postJSON(e, "/auth/apple", `{"id_token":"x"}`), http.StatusTooManyRequests)
}

func TestApple_causeReachesRequestLog(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"invalid token", fmt.Errorf("%w: token is expired", ErrInvalidIDToken), http.StatusUnauthorized},
		{"internal", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			e := echo.New()
			e.Use(httpx.RequestLog(slog.New(slog.NewTextHandler(&buf, nil))))
			e.POST("/auth/apple", testAuthHandlers(&fakeAuthService{appleErr: tt.err}).Apple)

			rec := postJSON(e, "/auth/apple", `{"id_token":"secret-raw-token"}`)

			assertHTTPStatus(t, rec, tt.wantStatus)
			if strings.Contains(rec.Body.String(), tt.err.Error()) {
				t.Errorf("body leaks the cause: %s", rec.Body.String())
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
