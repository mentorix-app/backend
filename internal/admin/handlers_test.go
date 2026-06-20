package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestGrantAdmin_invalidUserID(t *testing.T) {
	e := echo.New()
	h := NewHandlers(nil, nil, "test-jwt-secret-at-least-32-characters-long")
	e.POST("/admin/users/:user_id/roles/admin", h.GrantAdmin)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/not-a-uuid/roles/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
