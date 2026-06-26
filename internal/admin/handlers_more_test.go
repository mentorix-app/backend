package admin

import (
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestNewService(t *testing.T) {
	svc := &Service{store: &fakeRoleStore{roles: map[uuid.UUID][]string{}}}
	if svc.store == nil {
		t.Fatal("expected store")
	}
}

func TestHandlers_Mount_registersRoute(t *testing.T) {
	svc := &Service{store: &fakeRoleStore{roles: map[uuid.UUID][]string{}}}
	h := NewHandlers(svc, nil, "test-jwt-secret-at-least-32-chars")
	e := echo.New()
	h.Mount(e)

	found := false
	for _, r := range e.Routes() {
		if r.Method == "POST" && r.Path == "/admin/users/:user_id/roles/admin" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("admin grant route not registered")
	}
}
