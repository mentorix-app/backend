package admin

import (
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/subscription"
)

func TestNewService(t *testing.T) {
	svc := &Service{plans: &fakePlanService{grants: map[uuid.UUID]subscription.Plan{}}}
	if svc.plans == nil {
		t.Fatal("expected plan service")
	}
}

func TestHandlers_Mount_registersRoutes(t *testing.T) {
	svc := &Service{plans: &fakePlanService{grants: map[uuid.UUID]subscription.Plan{}}}
	h := NewHandlers(svc, nil, "test-jwt-secret-at-least-32-chars")
	e := echo.New()
	h.Mount(e)

	var foundPut, foundDelete bool
	for _, r := range e.Routes() {
		if r.Path == "/admin/trainers/:user_id/plan" {
			switch r.Method {
			case "PUT":
				foundPut = true
			case "DELETE":
				foundDelete = true
			}
		}
	}
	if !foundPut || !foundDelete {
		t.Fatalf("plan routes not registered: put=%v delete=%v", foundPut, foundDelete)
	}
}
