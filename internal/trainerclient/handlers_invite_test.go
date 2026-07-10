package trainerclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
)

func TestHandlers_CreateInvite_unauthorized(t *testing.T) {
	h := testHandlers(nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/trainer/invites", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.CreateInvite(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}

func TestHandlers_ListClients_unauthorized(t *testing.T) {
	h := testHandlers(nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/trainer/clients", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.ListClients(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}

func TestHandlers_ListClients_invalidSortBy(t *testing.T) {
	h := testHandlers(nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/trainer/clients?sort_by=invalid", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.ListClients(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}
