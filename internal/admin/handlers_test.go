package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
)

const testJWTSecret = "test-jwt-secret-at-least-32-characters-long"

func TestGrantAdmin_invalidUserID(t *testing.T) {
	e := echo.New()
	h := NewHandlers(nil, nil, testJWTSecret)
	e.POST("/admin/users/:user_id/roles/admin", h.GrantAdmin)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/not-a-uuid/roles/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGrantAdmin_notFound(t *testing.T) {
	store := &fakeRoleStore{
		profiles: map[uuid.UUID]auth.UserProfile{},
		roles:    map[uuid.UUID][]string{},
	}
	svc := &Service{store: store}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	targetID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetID.String()+"/roles/admin", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(targetID.String())

	err := h.GrantAdmin(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound || he.Message != httpx.MsgUserNotFound {
		t.Fatalf("error = %v, want 404", err)
	}
}

func TestGrantAdmin_success(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	createdAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	store := &fakeRoleStore{
		profiles: map[uuid.UUID]auth.UserProfile{
			userID: {Email: "trainer@test.com", CreatedAt: createdAt},
		},
		roles: map[uuid.UUID][]string{
			userID: {auth.RoleTrainer},
		},
	}
	svc := &Service{store: store}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+userID.String()+"/roles/admin", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(userID.String())

	if err := h.GrantAdmin(c); err != nil {
		t.Fatalf("GrantAdmin: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.UserID != userID.String() || resp.Email != "trainer@test.com" {
		t.Fatalf("resp = %+v", resp)
	}
	if len(resp.Roles) != 2 {
		t.Fatalf("roles = %v, want 2", resp.Roles)
	}
}
