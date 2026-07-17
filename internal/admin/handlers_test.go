package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/subscription"
)

const testJWTSecret = "test-jwt-secret-at-least-32-characters-long"

func TestGrantPlan_freePlan(t *testing.T) {
	userID := uuid.New()
	plans := &fakePlanService{grants: map[uuid.UUID]subscription.Plan{userID: subscription.PlanElite}}
	svc := &Service{plans: plans}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/trainers/"+userID.String()+"/plan", strings.NewReader(`{"plan":"free"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(userID.String())

	if err := h.GrantPlan(c); err != nil {
		t.Fatalf("GrantPlan free: %v", err)
	}
	var resp planResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Subscription == nil || resp.Subscription.Plan != subscription.PlanFree {
		t.Fatalf("subscription = %+v, want free", resp.Subscription)
	}
}

func TestGrantPlan_invalidJSON(t *testing.T) {
	e := echo.New()
	h := NewHandlers(&Service{plans: &fakePlanService{grants: map[uuid.UUID]subscription.Plan{}}}, nil, testJWTSecret)
	targetID := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/trainers/"+targetID.String()+"/plan", strings.NewReader("not-json"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(targetID.String())

	err := h.GrantPlan(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestGrantPlan_invalidUserID(t *testing.T) {
	e := echo.New()
	h := NewHandlers(nil, nil, testJWTSecret)
	e.PUT("/admin/trainers/:user_id/plan", h.GrantPlan)

	req := httptest.NewRequest(http.MethodPut, "/admin/trainers/not-a-uuid/plan", strings.NewReader(`{"plan":"elite"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGrantPlan_internalError(t *testing.T) {
	plans := &fakePlanService{err: errors.New("db down")}
	svc := &Service{plans: plans}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	targetID := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/trainers/"+targetID.String()+"/plan", strings.NewReader(`{"plan":"elite"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(targetID.String())

	err := h.GrantPlan(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusInternalServerError {
		t.Fatalf("error = %v, want 500", err)
	}
}

func TestGrantPlan_invalidPlan(t *testing.T) {
	plans := &fakePlanService{err: subscription.ErrInvalidPlan}
	svc := &Service{plans: plans}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	targetID := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/trainers/"+targetID.String()+"/plan", strings.NewReader(`{"plan":"platinum"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(targetID.String())

	err := h.GrantPlan(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestGrantPlan_trainerNotFound(t *testing.T) {
	plans := &fakePlanService{err: subscription.ErrTrainerNotFound}
	svc := &Service{plans: plans}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	targetID := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/trainers/"+targetID.String()+"/plan", strings.NewReader(`{"plan":"elite"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(targetID.String())

	err := h.GrantPlan(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound {
		t.Fatalf("error = %v, want 404", err)
	}
}

func TestGrantPlan_success(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	plans := &fakePlanService{grants: map[uuid.UUID]subscription.Plan{}}
	svc := &Service{plans: plans}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/trainers/"+userID.String()+"/plan", strings.NewReader(`{"plan":"advance"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(userID.String())

	if err := h.GrantPlan(c); err != nil {
		t.Fatalf("GrantPlan: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp planResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.UserID != userID.String() {
		t.Fatalf("user_id = %s, want %s", resp.UserID, userID)
	}
	if resp.Subscription == nil || resp.Subscription.Plan != subscription.PlanAdvance {
		t.Fatalf("subscription = %+v, want advance plan", resp.Subscription)
	}
}

func TestRevokePlan_invalidUserID(t *testing.T) {
	e := echo.New()
	h := NewHandlers(nil, nil, testJWTSecret)
	req := httptest.NewRequest(http.MethodDelete, "/admin/trainers/not-a-uuid/plan", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues("not-a-uuid")

	err := h.RevokePlan(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestRevokePlan_trainerNotFound(t *testing.T) {
	plans := &fakePlanService{err: subscription.ErrTrainerNotFound}
	svc := &Service{plans: plans}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	targetID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/admin/trainers/"+targetID.String()+"/plan", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(targetID.String())

	err := h.RevokePlan(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound {
		t.Fatalf("error = %v, want 404", err)
	}
}

func TestRevokePlan_success(t *testing.T) {
	userID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	plans := &fakePlanService{grants: map[uuid.UUID]subscription.Plan{userID: subscription.PlanElite}}
	svc := &Service{plans: plans}
	h := NewHandlers(svc, nil, testJWTSecret)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/admin/trainers/"+userID.String()+"/plan", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("user_id")
	c.SetParamValues(userID.String())

	if err := h.RevokePlan(c); err != nil {
		t.Fatalf("RevokePlan: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp planResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Subscription == nil || resp.Subscription.Plan != subscription.PlanFree {
		t.Fatalf("subscription = %+v, want free plan after revoke", resp.Subscription)
	}
}
