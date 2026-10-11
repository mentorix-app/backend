package apicheck

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestMountRoutes_unknownPathsAnswer404(t *testing.T) {
	e := echo.New()
	MountRoutes(e, nil)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"unknown root path without token", http.MethodGet, "/does-not-exist", http.StatusNotFound},
		{"unknown nested path without token", http.MethodGet, "/auth/does-not-exist", http.StatusNotFound},
		{"protected path without token", http.MethodGet, "/auth/me", http.StatusUnauthorized},
		{"protected profile update without token", http.MethodPatch, "/auth/me", http.StatusUnauthorized},
		{"protected logout-all without token", http.MethodPost, "/auth/logout-all", http.StatusUnauthorized},
		{"health stays open", http.MethodGet, "/health", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.want {
				t.Fatalf("%s %s = %d, want %d; body = %s", tt.method, tt.path, rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

// The contract mount goes through app.Mount, so errors from the real routes and
// middleware carry the code that the OpenAPI ErrorResponse schema promises.
func TestMountRoutes_errorBodiesCarryCodes(t *testing.T) {
	e := echo.New()
	MountRoutes(e, nil)

	tests := []struct {
		name          string
		method        string
		path          string
		authorization string
		want          string
	}{
		{"unknown path", http.MethodGet, "/does-not-exist", "", `{"code":"not_found","message":"Not Found"}`},
		{"wrong method", http.MethodDelete, "/health", "", `{"code":"method_not_allowed","message":"Method Not Allowed"}`},
		{"no token", http.MethodGet, "/auth/me", "", `{"code":"missing_authorization","message":"missing authorization"}`},
		{"garbage token", http.MethodGet, "/auth/me", "Bearer nope", `{"code":"invalid_token","message":"invalid token"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.authorization != "" {
				req.Header.Set(echo.HeaderAuthorization, tt.authorization)
			}
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Fatalf("%s %s: body = %s, want %s", tt.method, tt.path, got, tt.want)
			}
		})
	}
}
