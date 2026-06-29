//go:build integration

package storetest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
)

func TestTrainerMiddleware_allowsRegisteredTrainer(t *testing.T) {
	pool := NewPool(t)
	ctx := t.Context()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := auth.NewStore(pool).RegisterTrainerEmailPassword(ctx, "middleware-trainer@test.com", hash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	e := echo.New()
	called := false
	h := auth.TrainerMiddleware(pool)(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, userID)

	if err := h(c); err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if !called {
		t.Fatal("expected handler to run")
	}
}

func TestAdminMiddleware_deniesTrainerWithoutAdmin(t *testing.T) {
	pool := NewPool(t)
	ctx := t.Context()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := auth.NewStore(pool).RegisterTrainerEmailPassword(ctx, "middleware-no-admin@test.com", hash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	e := echo.New()
	h := auth.AdminMiddleware(pool)(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, userID)

	err = h(c)
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusForbidden {
		t.Fatalf("error = %v, want 403", err)
	}
}
