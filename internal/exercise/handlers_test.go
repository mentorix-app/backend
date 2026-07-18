package exercise

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/subscription"
)

type fakeExerciseStore struct {
	listResult      ListResult
	listErr         error
	getEx           Exercise
	getErr          error
	createEx        Exercise
	createErr       error
	updateEx        Exercise
	updateErr       error
	deleteManyCount int64
	deleteManyErr   error
	trainerID       uuid.UUID
}

func (f *fakeExerciseStore) List(context.Context, Viewer, ListParams) (ListResult, error) {
	return f.listResult, f.listErr
}

func (f *fakeExerciseStore) GetByID(context.Context, uuid.UUID) (Exercise, error) {
	if f.getErr != nil {
		return Exercise{}, f.getErr
	}
	return f.getEx, nil
}

func (f *fakeExerciseStore) Create(context.Context, uuid.UUID, *uuid.UUID, UpsertInput) (Exercise, error) {
	if f.createErr != nil {
		return Exercise{}, f.createErr
	}
	return f.createEx, nil
}

func (f *fakeExerciseStore) Update(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, UpsertInput) (Exercise, error) {
	if f.updateErr != nil {
		return Exercise{}, f.updateErr
	}
	return f.updateEx, nil
}

func (f *fakeExerciseStore) DeleteMany(context.Context, uuid.UUID, *uuid.UUID, []uuid.UUID) (int64, error) {
	if f.deleteManyErr != nil {
		return 0, f.deleteManyErr
	}
	return f.deleteManyCount, nil
}

func (f *fakeExerciseStore) TrainerIDForUser(context.Context, uuid.UUID) (uuid.UUID, error) {
	if f.trainerID == uuid.Nil {
		f.trainerID = uuid.New()
	}
	return f.trainerID, nil
}

func TestNewService(t *testing.T) {
	svc := NewService(nil)
	if svc == nil || svc.store == nil {
		t.Fatal("expected service")
	}
}

func TestNewHandlers(t *testing.T) {
	h := NewHandlers(&Service{store: &stubExerciseStore{}}, nil, "test-jwt-secret-at-least-32-chars")
	if h == nil {
		t.Fatal("expected handlers")
	}
}

func testExerciseHandlers(store *fakeExerciseStore) *Handlers {
	svc := NewServiceWithStore(store, &fakeRoles{admins: map[uuid.UUID]bool{}}, nil)
	return &Handlers{svc: svc}
}

func TestHandlers_Mount_registersRoutes(t *testing.T) {
	h := testExerciseHandlers(&fakeExerciseStore{})
	e := echo.New()
	h.Mount(e)
	found := map[string]bool{}
	for _, r := range e.Routes() {
		found[r.Method+" "+r.Path] = true
	}
	if !found["GET /exercises"] || !found["POST /exercises"] {
		t.Fatalf("routes missing: %v", found)
	}
}

func sampleExercise() Exercise {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	return Exercise{
		ID:          uuid.New(),
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        ExerciseTypeStrength,
		MuscleGroup: MuscleGroupLegs,
		Difficulty:  DifficultyIntermediate,
		CreatedAt:   now,
		ModifiedAt:  now,
		Scope:       ScopeGlobal,
	}
}

func validUpsertJSON() string {
	return `{
		"name":"Squat",
		"name_ru":"Присед",
		"type":"strength",
		"muscle_group":"legs",
		"difficulty":"intermediate"
	}`
}

func TestList(t *testing.T) {
	store := &fakeExerciseStore{
		listResult: ListResult{
			Items:      []Exercise{sampleExercise()},
			Pagination: Pagination{Page: 1, Limit: 20, Total: 1},
		},
	}
	e := echo.New()
	h := testExerciseHandlers(store)

	req := httptest.NewRequest(http.MethodGet, "/exercises?page=1&limit=20&sort_by=name&sort_order=asc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	if err := h.List(c); err != nil {
		t.Fatalf("List: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestList_invalidParams(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{})

	req := httptest.NewRequest(http.MethodGet, "/exercises?sort_by=invalid", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.List(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestList_serviceError(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{listErr: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/exercises", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.List(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusInternalServerError {
		t.Fatalf("error = %v, want 500", err)
	}
}

func TestGet(t *testing.T) {
	ex := sampleExercise()
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{getEx: ex})
	e.GET("/exercises/:id", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/exercises/"+ex.ID.String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(ex.ID.String())
	c.Set(auth.ContextUserIDKey, uuid.New())

	if err := h.Get(c); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestGet_invalidID(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{})
	e.GET("/exercises/:id", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/exercises/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("not-a-uuid")
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.Get(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestGet_notFound(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{getErr: pgx.ErrNoRows})
	e.GET("/exercises/:id", h.Get)

	id := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/exercises/"+id.String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.Get(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound || he.Message != httpx.MsgExerciseNotFound {
		t.Fatalf("error = %v, want 404", err)
	}
}

func TestCreate(t *testing.T) {
	ex := sampleExercise()
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{createEx: ex})
	req := httptest.NewRequest(http.MethodPost, "/exercises", strings.NewReader(validUpsertJSON()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	if err := h.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func TestCreate_missingUser(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{})
	req := httptest.NewRequest(http.MethodPost, "/exercises", strings.NewReader(validUpsertJSON()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Create(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}

func TestCreate_invalidJSON(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{})
	req := httptest.NewRequest(http.MethodPost, "/exercises", strings.NewReader("not-json"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.Create(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestCreate_quotaExceeded(t *testing.T) {
	qe := &subscription.QuotaError{Resource: subscription.ResourceExercises, Plan: subscription.PlanFree, Limit: 10, Usage: 10}
	store := &fakeExerciseStore{createErr: qe}
	e := echo.New()
	h := testExerciseHandlers(store)
	req := httptest.NewRequest(http.MethodPost, "/exercises", strings.NewReader(validUpsertJSON()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.Create(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusConflict {
		t.Fatalf("error = %v, want 409", err)
	}
}

func TestCreate_validationError(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{})
	body := `{"name":"","type":"strength","muscle_group":"legs","difficulty":"intermediate"}`
	req := httptest.NewRequest(http.MethodPost, "/exercises", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.Create(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestUpdate(t *testing.T) {
	ex := sampleExercise()
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{updateEx: ex})
	req := httptest.NewRequest(http.MethodPut, "/exercises/"+ex.ID.String(), strings.NewReader(validUpsertJSON()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(ex.ID.String())
	c.Set(auth.ContextUserIDKey, uuid.New())

	if err := h.Update(c); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestUpdate_notFound(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{updateErr: pgx.ErrNoRows})
	id := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/exercises/"+id.String(), strings.NewReader(validUpsertJSON()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.Update(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusNotFound {
		t.Fatalf("error = %v, want 404", err)
	}
}

func TestUpdate_validationError(t *testing.T) {
	ex := sampleExercise()
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{})
	req := httptest.NewRequest(http.MethodPut, "/exercises/"+ex.ID.String(), strings.NewReader(`{"name":"","type":"strength","muscle_group":"legs","difficulty":"beginner"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(ex.ID.String())
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.Update(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestUpdate_internalError(t *testing.T) {
	ex := sampleExercise()
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{updateErr: errors.New("db down")})
	req := httptest.NewRequest(http.MethodPut, "/exercises/"+ex.ID.String(), strings.NewReader(validUpsertJSON()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(ex.ID.String())
	c.Set(auth.ContextUserIDKey, uuid.New())

	err := h.Update(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusInternalServerError {
		t.Fatalf("error = %v, want 500", err)
	}
}

func TestDeleteMany(t *testing.T) {
	id1 := uuid.New().String()
	id2 := uuid.New().String()

	tests := []struct {
		name       string
		body       string
		store      *fakeExerciseStore
		wantStatus int
		wantCount  int64
	}{
		{
			name:       "success",
			body:       `{"ids":["` + id1 + `","` + id2 + `"]}`,
			store:      &fakeExerciseStore{deleteManyCount: 2},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "empty ids",
			body:       `{"ids":[]}`,
			store:      &fakeExerciseStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid uuid",
			body:       `{"ids":["not-a-uuid"]}`,
			store:      &fakeExerciseStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "service error",
			body:       `{"ids":["` + id1 + `"]}`,
			store:      &fakeExerciseStore{deleteManyErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			h := testExerciseHandlers(tt.store)

			req := httptest.NewRequest(http.MethodDelete, "/exercises", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(auth.ContextUserIDKey, uuid.New())

			if err := h.DeleteMany(c); err != nil {
				he, ok := err.(*echo.HTTPError)
				if !ok {
					t.Fatalf("DeleteMany: %v", err)
				}
				if he.Code != tt.wantStatus {
					t.Fatalf("status = %d, want %d", he.Code, tt.wantStatus)
				}
				return
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var resp deleteManyResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.DeletedCount != tt.wantCount {
				t.Fatalf("deleted_count = %d, want %d", resp.DeletedCount, tt.wantCount)
			}
		})
	}
}

func TestUpdate_missingUser(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{})
	req := httptest.NewRequest(http.MethodPut, "/exercises/"+uuid.New().String(), strings.NewReader(validUpsertJSON()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())

	err := h.Update(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}

func TestDeleteMany_missingUser(t *testing.T) {
	e := echo.New()
	h := testExerciseHandlers(&fakeExerciseStore{})
	req := httptest.NewRequest(http.MethodDelete, "/exercises", strings.NewReader(`{"ids":["`+uuid.New().String()+`"]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.DeleteMany(c)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}
