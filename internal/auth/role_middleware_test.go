package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestTrainerMiddleware_unauthorizedWithoutUser(t *testing.T) {
	e := echo.New()
	h := TrainerMiddleware(nil)(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
}

func TestAdminMiddleware_unauthorizedWithoutUser(t *testing.T) {
	e := echo.New()
	h := AdminMiddleware(nil)(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
}
