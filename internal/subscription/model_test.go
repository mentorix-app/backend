package subscription

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestPlanValidAndRank(t *testing.T) {
	if !PlanFree.Valid() || !PlanAdvance.Valid() || !PlanElite.Valid() {
		t.Fatal("expected known plans to be valid")
	}
	if Plan("platinum").Valid() {
		t.Fatal("unknown plan must be invalid")
	}
	if PlanElite.rank() <= PlanAdvance.rank() || PlanAdvance.rank() <= PlanFree.rank() {
		t.Fatal("expected elite > advance > free ranks")
	}
}

func TestCatalog(t *testing.T) {
	items := Catalog()
	if len(items) != 3 {
		t.Fatalf("len = %d, want 3", len(items))
	}
	if items[0].Code != PlanFree || items[1].Code != PlanAdvance || items[2].Code != PlanElite {
		t.Fatalf("order = %+v", items)
	}
	if items[0].Limits.Exercises == nil || *items[0].Limits.Exercises != 10 {
		t.Fatalf("free exercises limit = %v, want 10", items[0].Limits.Exercises)
	}
	if items[2].Limits.Exercises != nil {
		t.Fatal("elite exercises must be unlimited")
	}
}

func TestPermissions_underLimit(t *testing.T) {
	limits := planLimits(PlanFree)
	usage := Usage{Exercises: 5, ActivePrograms: 1, ActiveClients: 1}
	p := permissions(limits, usage)
	if !p.CanCreateExercise || !p.CanEditExercises {
		t.Fatalf("permissions = %+v, want create+edit allowed", p)
	}
}

func TestPermissions_atLimit(t *testing.T) {
	limits := planLimits(PlanFree)
	usage := Usage{Exercises: 10, ActivePrograms: 3, ActiveClients: 3}
	p := permissions(limits, usage)
	if p.CanCreateExercise || p.CanCreateProgram || p.CanCreateInvite {
		t.Fatalf("create flags must be false at limit: %+v", p)
	}
	if !p.CanEditExercises || !p.CanEditPrograms || !p.CanManageClients {
		t.Fatalf("mutate flags must stay true at exact limit: %+v", p)
	}
}

func TestPermissions_overLimit(t *testing.T) {
	limits := planLimits(PlanFree)
	usage := Usage{Exercises: 11, ActivePrograms: 4, ActiveClients: 4}
	p := permissions(limits, usage)
	if p.CanCreateExercise || p.CanEditExercises {
		t.Fatalf("over-limit must be fully read-only for exercises: %+v", p)
	}
}

func TestPermissions_unlimited(t *testing.T) {
	limits := planLimits(PlanElite)
	usage := Usage{Exercises: 1000, ActivePrograms: 1000, ActiveClients: 40}
	p := permissions(limits, usage)
	if !p.CanCreateExercise || !p.CanCreateProgram {
		t.Fatalf("unlimited resources must allow create: %+v", p)
	}
	if !p.CanCreateInvite {
		t.Fatalf("elite clients under 50 must allow invite: %+v", p)
	}
}

func TestQuotaErrorAndHTTP(t *testing.T) {
	qe := &QuotaError{Resource: ResourceExercises, Plan: PlanFree, Limit: 10, Usage: 10}
	if qe.Error() == "" {
		t.Fatal("expected error string")
	}
	he := QuotaHTTPError(qe)
	if he.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", he.Code)
	}
	body, ok := he.Message.(quotaErrorBody)
	if !ok || body.Error != "quota_exceeded" || body.Resource != ResourceExercises {
		t.Fatalf("body = %#v", he.Message)
	}
}

func TestHandlers_ListPlans(t *testing.T) {
	h := NewHandlers(func(next echo.HandlerFunc) echo.HandlerFunc { return next })
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/plans", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ListPlans(c); err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp planListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(resp.Items))
	}
}

func TestHandlers_Mount(t *testing.T) {
	h := NewHandlers(func(next echo.HandlerFunc) echo.HandlerFunc { return next })
	e := echo.New()
	h.Mount(e)
	found := false
	for _, r := range e.Routes() {
		if r.Method == "GET" && r.Path == "/plans" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("/plans not registered")
	}
}

func TestLimitAndUsageFor(t *testing.T) {
	if got := limitFor(PlanFree, ResourceExercises); got == nil || *got != 10 {
		t.Fatalf("free exercises = %v", got)
	}
	if got := limitFor(PlanElite, ResourcePrograms); got != nil {
		t.Fatalf("elite programs must be unlimited, got %v", got)
	}
	if got := limitFor(PlanFree, Resource("unknown")); got != nil {
		t.Fatalf("unknown resource limit = %v, want nil", got)
	}
	u := Usage{Exercises: 1, ActivePrograms: 2, ActiveClients: 3}
	if usageFor(u, ResourceClients) != 3 {
		t.Fatal("usage clients mismatch")
	}
	if usageFor(u, Resource("unknown")) != 0 {
		t.Fatal("unknown usage must be 0")
	}
}

func TestPlanLimitsAdvance(t *testing.T) {
	limits := planLimits(PlanAdvance)
	if limits.Exercises == nil || *limits.Exercises != 50 {
		t.Fatalf("advance exercises = %v", limits.Exercises)
	}
	if limits.ActivePrograms == nil || *limits.ActivePrograms != 15 {
		t.Fatalf("advance programs = %v", limits.ActivePrograms)
	}
	if limits.ActiveClients == nil || *limits.ActiveClients != 15 {
		t.Fatalf("advance clients = %v", limits.ActiveClients)
	}
	elite := planLimits(PlanElite)
	if elite.ActiveClients == nil || *elite.ActiveClients != 50 {
		t.Fatalf("elite clients = %v", elite.ActiveClients)
	}
}
