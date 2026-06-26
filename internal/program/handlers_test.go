package program

import (
	"context"
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
	"mentorix-backend/internal/exercise"
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

func publishableDetail(userID, programID uuid.UUID) Detail {
	d := sampleDetail(userID, programID)
	d.Name = "Strength Program"
	cat := CategoryMuscleGain
	diff := Difficulty(exercise.DifficultyBeginner)
	d.Category = &cat
	d.Difficulty = &diff
	sets, reps := 3, 10
	d.Days[0].Exercises = []DayExercise{{
		ID:         uuid.New(),
		ExerciseID: uuid.New(),
		Sets:       &sets,
		Reps:       &reps,
	}}
	return d
}

func sampleDetail(userID, programID uuid.UUID) Detail {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	dayID := uuid.New()
	return Detail{
		Program: Program{
			ID:         programID,
			CreatedBy:  userID,
			ModifiedBy: userID,
			Status:     StatusDraft,
			Name:       "",
			CreatedAt:  now,
			ModifiedAt: now,
		},
		Days: []Day{{
			ID:        dayID,
			DayNumber: 1,
			SortOrder: 1,
			CreatedAt: now,
		}},
	}
}

func TestNewHandlers(t *testing.T) {
	h := NewHandlers(&Service{store: &fakeProgramStore{}}, nil, "test-jwt-secret-at-least-32-chars")
	if h == nil {
		t.Fatal("expected handlers")
	}
}

func programHandler(store programStore) *Handlers {
	return &Handlers{svc: &Service{store: store}}
}

func TestHandlers_Mount_registersRoutes(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	h.Mount(e)
	found := map[string]bool{}
	for _, r := range e.Routes() {
		found[r.Method+" "+r.Path] = true
	}
	if !found["GET /programs"] || !found["POST /programs"] {
		t.Fatalf("routes missing: %v", found)
	}
}

func programContext(e *echo.Echo, method, path, body string, userID uuid.UUID, params map[string]string) (echo.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, userID)
	if len(params) > 0 {
		names := make([]string, 0, len(params))
		values := make([]string, 0, len(params))
		for _, key := range []string{"id", "day_id", "item_id"} {
			if v, ok := params[key]; ok {
				names = append(names, key)
				values = append(values, v)
			}
		}
		c.SetParamNames(names...)
		c.SetParamValues(values...)
	}
	return c, rec
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, want, rec.Body.String())
	}
}

func assertHTTPError(t *testing.T, err error, wantCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != wantCode {
		t.Fatalf("error = %v, want %d", err, wantCode)
	}
}

func TestHandlers_List_passesQueryParams(t *testing.T) {
	userID := uuid.New()
	store := &listProgramStore{}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodGet, "/programs?page=2&limit=10&sort_by=name&sort_order=asc&q=test&status=draft&category=muscle_gain&difficulty=beginner", "", userID, nil)

	if err := h.List(c); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
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
	h := programHandler(&listProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodGet, "/programs?sort_by=invalid", "", uuid.New(), nil)

	err := h.List(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_List_unauthorized(t *testing.T) {
	h := programHandler(&listProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/programs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.List(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_Create(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &fakeProgramStore{createDetail: sampleDetail(userID, programID)}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs", "", userID, nil)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
}

func TestHandlers_Get(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodGet, "/programs/"+programID.String(), "", userID, map[string]string{"id": programID.String()})

	if err := h.Get(c); err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_Get_invalidID(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodGet, "/programs/bad", "", uuid.New(), map[string]string{"id": "bad"})

	err := h.Get(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_Get_notFound(t *testing.T) {
	h := programHandler(&fakeProgramStore{err: ErrNotFound})
	e := echo.New()
	id := uuid.New()
	c, _ := programContext(e, http.MethodGet, "/programs/"+id.String(), "", uuid.New(), map[string]string{"id": id.String()})

	err := h.Get(c)
	assertHTTPError(t, err, http.StatusNotFound)
}

func TestHandlers_Get_forbidden(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(ownerID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail, isAdmin: false}
	h := programHandler(store)

	e := echo.New()
	c, _ := programContext(e, http.MethodGet, "/programs/"+programID.String(), "", otherID, map[string]string{"id": programID.String()})

	err := h.Get(c)
	assertHTTPError(t, err, http.StatusForbidden)
}

func TestHandlers_Update(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"name":"My Program","category":"muscle_gain","difficulty":"beginner"}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPatch, "/programs/"+programID.String(), body, userID, map[string]string{"id": programID.String()})

	if err := h.Update(c); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_Update_invalidJSON(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPatch, "/programs/"+uuid.New().String(), "not-json", uuid.New(), map[string]string{"id": uuid.New().String()})

	err := h.Update(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_Delete(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodDelete, "/programs/"+programID.String(), "", userID, map[string]string{"id": programID.String()})

	if err := h.Delete(c); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	assertStatus(t, rec, http.StatusNoContent)
}

func TestHandlers_Publish_validationError(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{
		program: detail.Program,
		detail:  detail,
	}
	h := programHandler(store)

	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/publish", "", userID, map[string]string{"id": programID.String()})

	err := h.Publish(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_Publish_conflict(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	detail.Status = StatusPublished
	store := &fakeProgramStore{
		program: detail.Program,
		detail:  detail,
	}
	h := programHandler(store)

	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/publish", "", userID, map[string]string{"id": programID.String()})

	err := h.Publish(c)
	assertHTTPError(t, err, http.StatusConflict)
}

func TestHandlers_Publish_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := publishableDetail(userID, programID)
	store := &publishStore{fakeProgramStore: fakeProgramStore{
		program: detail.Program,
		detail:  detail,
	}}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/publish", "", userID, map[string]string{"id": programID.String()})

	if err := h.Publish(c); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_Archive_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := publishableDetail(userID, programID)
	detail.Status = StatusPublished
	store := &archiveStore{fakeProgramStore: fakeProgramStore{
		program: detail.Program,
		detail:  detail,
	}}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/archive", "", userID, map[string]string{"id": programID.String()})

	if err := h.Archive(c); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_AddDay(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/days", "", userID, map[string]string{"id": programID.String()})

	if err := h.AddDay(c); err != nil {
		t.Fatalf("AddDay: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteDay(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	dayID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/days/"+dayID.String(), "", userID, map[string]string{
		"id":     programID.String(),
		"day_id": dayID.String(),
	})

	if err := h.DeleteDay(c); err != nil {
		t.Fatalf("DeleteDay: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_AddDayExercise(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	dayID := uuid.New()
	exerciseID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"exercise_id":"` + exerciseID.String() + `","sets":3,"reps":10}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/days/"+dayID.String()+"/exercises", body, userID, map[string]string{
		"id":     programID.String(),
		"day_id": dayID.String(),
	})

	if err := h.AddDayExercise(c); err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
}

func TestHandlers_AddDayExercise_invalidExerciseID(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	body := `{"exercise_id":"bad","sets":3,"reps":10}`
	c, _ := programContext(e, http.MethodPost, "/programs/"+uuid.New().String()+"/days/"+uuid.New().String()+"/exercises", body, uuid.New(), map[string]string{
		"id":     uuid.New().String(),
		"day_id": uuid.New().String(),
	})

	err := h.AddDayExercise(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_UpdateDayExercise(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	dayID := uuid.New()
	itemID := uuid.New()
	exerciseID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"exercise_id":"` + exerciseID.String() + `","sets":4,"reps":8}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/days/"+dayID.String()+"/exercises/"+itemID.String(), body, userID, map[string]string{
		"id":      programID.String(),
		"day_id":  dayID.String(),
		"item_id": itemID.String(),
	})

	if err := h.UpdateDayExercise(c); err != nil {
		t.Fatalf("UpdateDayExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteDayExercise(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	dayID := uuid.New()
	itemID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/days/"+dayID.String()+"/exercises/"+itemID.String(), "", userID, map[string]string{
		"id":      programID.String(),
		"day_id":  dayID.String(),
		"item_id": itemID.String(),
	})

	if err := h.DeleteDayExercise(c); err != nil {
		t.Fatalf("DeleteDayExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteDayExercise_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, err: ErrNotFound}
	h := programHandler(store)

	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/days/"+uuid.New().String()+"/exercises/"+uuid.New().String(), "", userID, map[string]string{
		"id":      programID.String(),
		"day_id":  uuid.New().String(),
		"item_id": uuid.New().String(),
	})

	err := h.DeleteDayExercise(c)
	assertHTTPError(t, err, http.StatusNotFound)
}

type publishStore struct {
	fakeProgramStore
}

func (p *publishStore) SetStatus(_ context.Context, _ uuid.UUID, _ uuid.UUID, status Status) (Detail, error) {
	if p.err != nil {
		return Detail{}, p.err
	}
	d := p.detail
	d.Status = status
	return d, nil
}

type archiveStore struct {
	fakeProgramStore
}

func (a *archiveStore) SetStatus(_ context.Context, _ uuid.UUID, _ uuid.UUID, status Status) (Detail, error) {
	if a.err != nil {
		return Detail{}, a.err
	}
	d := a.detail
	d.Status = status
	return d, nil
}

func TestHandlers_Get_mapsServiceErrors(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	tests := []struct {
		name  string
		store *fakeProgramStore
		code  int
	}{
		{
			name:  "not found",
			store: &fakeProgramStore{err: pgx.ErrNoRows},
			code:  http.StatusNotFound,
		},
		{
			name: "forbidden",
			store: &fakeProgramStore{
				isAdmin: false,
				program: Program{ID: programID, CreatedBy: uuid.New()},
			},
			code: http.StatusForbidden,
		},
		{
			name: "internal",
			store: &fakeProgramStore{
				program: Program{ID: programID, CreatedBy: userID},
				err:     errors.New("db down"),
			},
			code: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := programHandler(tt.store)
			e := echo.New()
			c, _ := programContext(e, http.MethodGet, "/programs/"+programID.String(), "", userID, map[string]string{"id": programID.String()})
			err := h.Get(c)
			assertHTTPError(t, err, tt.code)
		})
	}
}

func TestHandlers_Publish_mapsValidationError(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail: Detail{
			Program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
			Days:    []Day{{DayNumber: 1, Exercises: []DayExercise{}}},
		},
	}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/publish", "", userID, map[string]string{"id": programID.String()})
	err := h.Publish(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_Archive_mapsStatusConflict(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/archive", "", userID, map[string]string{"id": programID.String()})
	err := h.Archive(c)
	assertHTTPError(t, err, http.StatusConflict)
}
