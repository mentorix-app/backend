package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestTokenResponse_refreshExpiresAt(t *testing.T) {
	refreshExp := time.Date(2026, 11, 19, 10, 30, 0, 0, time.FixedZone("UTC+3", 3*3600))
	userID := uuid.New()
	issued := IssuedAuth{UserID: userID, Email: "u@example.com", RefreshExpires: refreshExp}
	svc := &fakeAuthService{
		refreshIssued: issued, googleIssued: issued, appleIssued: issued, linkIssued: issued,
	}
	h := testAuthHandlers(svc)
	e := echo.New()
	e.POST("/auth/refresh", h.Refresh)
	e.POST("/auth/google", h.Google)
	e.POST("/auth/apple", h.Apple)

	cookieRefresh := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: "mentorix_refresh", Value: "cookie-token"})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	tests := []struct {
		name string
		rec  *httptest.ResponseRecorder
	}{
		{"refresh (cookie mode)", cookieRefresh()},
		{"refresh (body mode)", postJSON(e, "/auth/refresh", `{"refresh_token":"body-token"}`)},
		{"google (body mode)", postJSON(e, "/auth/google", idTokenBody)},
		{"apple (body mode)", postJSON(e, "/auth/apple", appleIDTokenBody)},
		{"telegram link (body mode)", postLink(t, linkEcho(h), userID, linkBody)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.rec.Code != http.StatusOK && tt.rec.Code != http.StatusCreated {
				t.Fatalf("status = %d; body = %s", tt.rec.Code, tt.rec.Body)
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(tt.rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			field, ok := raw["refresh_expires_at"]
			if !ok {
				t.Fatalf("refresh_expires_at missing from %s", tt.rec.Body)
			}
			var got string
			if err := json.Unmarshal(field, &got); err != nil {
				t.Fatalf("refresh_expires_at is not a string: %s", field)
			}
			if want := refreshExp.UTC().Format(time.RFC3339); got != want {
				t.Errorf("refresh_expires_at = %q, want %q", got, want)
			}
		})
	}
}
