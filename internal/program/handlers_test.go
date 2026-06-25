package program

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
)

type listProgramStore struct {
	fakeProgramStore
	lastParams ListParams
}

func (s *listProgramStore) List(_ context.Context, params ListParams) (ListResult, error) {
	s.lastParams = params
	return ListResult{Items: []Program{}}, nil
}

func (s *listProgramStore) IsAdmin(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

func TestHandlers_List_passesQueryParams(t *testing.T) {
	userID := uuid.New()
	store := &listProgramStore{}
	svc := &Service{store: store}
	h := &Handlers{svc: svc}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/programs?page=2&limit=10&sort_by=name&sort_order=asc&q=test&status=draft&category=muscle_gain&difficulty=beginner", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/programs")
	c.Set(auth.ContextUserIDKey, userID)

	if err := h.List(c); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if store.lastParams.Page != 2 || store.lastParams.Limit != 10 {
		t.Fatalf("page/limit = %d/%d", store.lastParams.Page, store.lastParams.Limit)
	}
	if store.lastParams.Query != "test" {
		t.Fatalf("query = %q", store.lastParams.Query)
	}
	if len(store.lastParams.Statuses) != 1 || store.lastParams.Statuses[0] != StatusDraft {
		t.Fatalf("statuses = %v", store.lastParams.Statuses)
	}
	if store.lastParams.Category == nil || *store.lastParams.Category != CategoryMuscleGain {
		t.Fatalf("category = %v", store.lastParams.Category)
	}
}

func TestHandlers_List_invalidSortBy(t *testing.T) {
	h := &Handlers{svc: &Service{store: &listProgramStore{}}}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/programs?sort_by=invalid", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.List(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}
