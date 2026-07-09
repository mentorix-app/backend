package program

import (
	"context"
	"errors"
	"fmt"
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

type errListProgramStore struct {
	fakeProgramStore
}

func (s *errListProgramStore) List(context.Context, ListParams) (ListResult, error) {
	return ListResult{}, errors.New("list failed")
}

func (s *listProgramStore) List(_ context.Context, params ListParams) (ListResult, error) {
	s.lastParams = params
	return ListResult{Items: []Program{}}, nil
}

func programHandler(store programStore) *Handlers {
	return programHandlerWithRoles(store, &fakeRoleQuerier{isAdmin: true})
}

func programHandlerWithRoles(store programStore, roles *fakeRoleQuerier) *Handlers {
	return &Handlers{svc: &Service{store: store, roles: roles}}
}

func publishableDetail(userID, programID uuid.UUID) Detail {
	d := sampleDetail(userID, programID)
	d.Name = "Strength Program"
	cat := CategoryMuscleGain
	diff := Difficulty(exercise.DifficultyBeginner)
	d.Category = &cat
	d.Difficulty = &diff
	sets, reps := 3, 10
	blockID := uuid.New()
	d.Weeks[0].Days[0].Blocks = []DayBlock{{
		ID:        blockID,
		BlockType: BlockTypeSingle,
		Exercises: []DayExercise{{
			ID:         uuid.New(),
			ExerciseID: uuid.New(),
			Sets:       &sets,
			Reps:       &reps,
		}},
	}}
	return d
}

func sampleDetail(userID, programID uuid.UUID) Detail {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	weekID := uuid.New()
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
		Weeks: []Week{{
			ID:         weekID,
			WeekNumber: 1,
			SortOrder:  1,
			CreatedAt:  now,
			Days: []Day{{
				ID:        dayID,
				DayNumber: 1,
				SortOrder: 1,
				CreatedAt: now,
			}},
		}},
	}
}

func TestNewHandlers(t *testing.T) {
	h := NewHandlers(testService(&fakeProgramStore{}, nil), nil, "test-jwt-secret-at-least-32-chars")
	if h == nil {
		t.Fatal("expected handlers")
	}
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
	if !found["POST /programs/:id/publish-update"] || !found["GET /programs/:id/assignments"] {
		t.Fatalf("assignment/version routes missing: %v", found)
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
		for _, key := range []string{"id", "week_id", "day_id", "block_id", "item_id", "version_id", "client_user_id"} {
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

func TestHandlers_List_storeError(t *testing.T) {
	h := programHandler(&errListProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodGet, "/programs", "", uuid.New(), nil)
	err := h.List(c)
	assertHTTPError(t, err, http.StatusInternalServerError)
}

func TestHandlers_Create_storeError(t *testing.T) {
	h := programHandler(&errCreateStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs", "", uuid.New(), nil)
	err := h.Create(c)
	assertHTTPError(t, err, http.StatusInternalServerError)
}

type errCreateStore struct {
	fakeProgramStore
}

func (errCreateStore) CreateDraft(context.Context, uuid.UUID) (Detail, error) {
	return Detail{}, errors.New("create failed")
}

type errUpdateStore struct {
	fakeProgramStore
}

func (errUpdateStore) Update(context.Context, uuid.UUID, uuid.UUID, UpdateInput) (Detail, error) {
	return Detail{}, errors.New("update failed")
}

func TestHandlers_Update_storeError(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	h := programHandler(&errUpdateStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
	})
	e := echo.New()
	body := `{"name":"x"}`
	c, _ := programContext(e, http.MethodPatch, "/programs/"+programID.String(), body, userID, map[string]string{"id": programID.String()})
	err := h.Update(c)
	assertHTTPError(t, err, http.StatusInternalServerError)
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

func TestHandlers_mapProgramError_internal(t *testing.T) {
	err := mapProgramError(errors.New("boom"))
	if err.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", err.Code)
	}
}

func TestHandlers_ReorderWeeks_invalidJSON(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/programs/"+uuid.New().String()+"/weeks/reorder", "{", uuid.New(), map[string]string{
		"id": uuid.New().String(),
	})
	err := h.ReorderWeeks(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_ReorderWeeks_invalidWeekIDs(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	h := programHandler(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	})
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/reorder", `{"week_ids":["bad"]}`, userID, map[string]string{
		"id": programID.String(),
	})
	err := h.ReorderWeeks(c)
	assertHTTPError(t, err, http.StatusBadRequest)
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
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandlerWithRoles(store, &fakeRoleQuerier{isAdmin: false})

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

func TestHandlers_AddWeek(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks", "", userID, map[string]string{
		"id": programID.String(),
	})

	if err := h.AddWeek(c); err != nil {
		t.Fatalf("AddWeek: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteWeek_mapsLastWeek(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	store := &deleteWeekErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		deleteWeekErr: ErrLastWeek,
	}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/weeks/"+weekID.String(), "", userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
	})
	err := h.DeleteWeek(c)
	assertHTTPError(t, err, http.StatusConflict)
}

func TestHandlers_ReorderWeeks_mapsInvalidReorder(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &reorderWeeksErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: ErrInvalidReorder,
	}
	h := programHandler(store)
	body := `{"week_ids":["` + uuid.New().String() + `"]}`
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/reorder", body, userID, map[string]string{
		"id": programID.String(),
	})
	err := h.ReorderWeeks(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_ReorderWeeks_countMismatchMessage(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &reorderWeeksErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: fmt.Errorf("%w: week count mismatch", ErrInvalidReorder),
	}
	h := programHandler(store)
	body := `{"week_ids":["` + uuid.New().String() + `"]}`
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/reorder", body, userID, map[string]string{
		"id": programID.String(),
	})
	err := h.ReorderWeeks(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
	msg, ok := he.Message.(string)
	if !ok || !strings.Contains(msg, "week count mismatch") {
		t.Fatalf("message = %v, want week count mismatch", he.Message)
	}
}

func TestHandlers_DeleteWeek_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)
	e := echo.New()
	c, rec := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/weeks/"+weekID.String(), "", userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
	})
	if err := h.DeleteWeek(c); err != nil {
		t.Fatalf("DeleteWeek: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_ReorderWeeks(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"week_ids":["` + weekID.String() + `"]}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/reorder", body, userID, map[string]string{
		"id": programID.String(),
	})

	if err := h.ReorderWeeks(c); err != nil {
		t.Fatalf("ReorderWeeks: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteDay_mapsLastDay(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	dayID := uuid.New()
	store := &deleteDayErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		deleteDayErr: ErrLastDay,
	}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/"+dayID.String(), "", userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
		"day_id":  dayID.String(),
	})
	err := h.DeleteDay(c)
	assertHTTPError(t, err, http.StatusConflict)
}

func TestHandlers_AddDay_mapsMaxDays(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	store := &addDayErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		addDayErr: ErrMaxDaysPerWeek,
	}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days", "", userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
	})
	err := h.AddDay(c)
	assertHTTPError(t, err, http.StatusConflict)
}

func TestHandlers_AddDay_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	store := &addDayErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		addDayErr: ErrNotFound,
	}
	h := programHandler(store)
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days", "", userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
	})
	err := h.AddDay(c)
	assertHTTPError(t, err, http.StatusNotFound)
}

func TestHandlers_ReorderDays_mapsInvalidReorder(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	store := &reorderDaysErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: ErrInvalidReorder,
	}
	h := programHandler(store)
	body := `{"day_ids":["` + uuid.New().String() + `"]}`
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/reorder", body, userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
	})
	err := h.ReorderDays(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_ReorderBlockExercises_mapsInvalidReorder(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	blockID := uuid.New()
	store := &reorderBlockExercisesErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: ErrInvalidReorder,
	}
	h := programHandler(store)
	body := `{"exercise_item_ids":["` + uuid.New().String() + `"]}`
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises/reorder", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	err := h.ReorderBlockExercises(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_ReorderDays(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	dayID := detail.Weeks[0].Days[0].ID
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"day_ids":["` + dayID.String() + `"]}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/reorder", body, userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
	})

	if err := h.ReorderDays(c); err != nil {
		t.Fatalf("ReorderDays: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_ReorderBlockExercises(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	blockID := uuid.New()
	itemID := uuid.New()
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"exercise_item_ids":["` + itemID.String() + `"]}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises/reorder", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})

	if err := h.ReorderBlockExercises(c); err != nil {
		t.Fatalf("ReorderBlockExercises: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_AddDay(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days", "", userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
	})

	if err := h.AddDay(c); err != nil {
		t.Fatalf("AddDay: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteDay(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	dayID := detail.Weeks[0].Days[0].ID
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/"+dayID.String(), "", userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
		"day_id":  dayID.String(),
	})

	if err := h.DeleteDay(c); err != nil {
		t.Fatalf("DeleteDay: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_CreateDayBlock_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"block_type":"single"}`)), httptest.NewRecorder())
	c.SetParamNames("id", "week_id", "day_id")
	c.SetParamValues(uuid.New().String(), uuid.New().String(), uuid.New().String())
	assertHTTPError(t, h.CreateDayBlock(c), http.StatusUnauthorized)
}

func TestHandlers_CreateDayBlock_invalidJSON(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/", "{", userID, map[string]string{
		"id": uuid.New().String(), "week_id": uuid.New().String(), "day_id": uuid.New().String(),
	})
	assertHTTPError(t, h.CreateDayBlock(c), http.StatusBadRequest)
}

func TestHandlers_ReorderDays_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"day_ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("id", "week_id")
	c.SetParamValues(uuid.New().String(), uuid.New().String())
	assertHTTPError(t, h.ReorderDays(c), http.StatusUnauthorized)
}

func TestHandlers_ReorderDays_invalidJSON(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/", "{", userID, map[string]string{
		"id": uuid.New().String(), "week_id": uuid.New().String(),
	})
	assertHTTPError(t, h.ReorderDays(c), http.StatusBadRequest)
}

func TestHandlers_ReorderBlockExercises_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"exercise_item_ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("id", "week_id", "block_id")
	c.SetParamValues(uuid.New().String(), uuid.New().String(), uuid.New().String())
	assertHTTPError(t, h.ReorderBlockExercises(c), http.StatusUnauthorized)
}

func TestHandlers_CreateDayBlock_nullSetsReps(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	exerciseID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	dayID := detail.Weeks[0].Days[0].ID
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"block_type":"single","exercise":{"exercise_id":"` + exerciseID.String() + `","sets":null,"reps":null}}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/"+dayID.String()+"/blocks", body, userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
		"day_id":  dayID.String(),
	})

	if err := h.CreateDayBlock(c); err != nil {
		t.Fatalf("CreateDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
}

func TestHandlers_CreateDayBlock(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	exerciseID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	dayID := detail.Weeks[0].Days[0].ID
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"block_type":"single","exercise":{"exercise_id":"` + exerciseID.String() + `","sets":3,"reps":10}}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/"+dayID.String()+"/blocks", body, userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
		"day_id":  dayID.String(),
	})

	if err := h.CreateDayBlock(c); err != nil {
		t.Fatalf("CreateDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
}

func TestHandlers_CreateDayBlock_invalidExerciseID(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	body := `{"block_type":"single","exercise":{"exercise_id":"bad","sets":3,"reps":10}}`
	c, _ := programContext(e, http.MethodPost, "/programs/"+uuid.New().String()+"/weeks/"+uuid.New().String()+"/days/"+uuid.New().String()+"/blocks", body, uuid.New(), map[string]string{
		"id":      uuid.New().String(),
		"week_id": uuid.New().String(),
		"day_id":  uuid.New().String(),
	})

	err := h.CreateDayBlock(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_UpdateBlockExercise(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	blockID := uuid.New()
	itemID := uuid.New()
	exerciseID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	body := `{"exercise_id":"` + exerciseID.String() + `","sets":4,"reps":8}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises/"+itemID.String(), body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
		"item_id":  itemID.String(),
	})

	if err := h.UpdateBlockExercise(c); err != nil {
		t.Fatalf("UpdateBlockExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteBlockExercise(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	blockID := uuid.New()
	itemID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	h := programHandler(store)

	e := echo.New()
	c, rec := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises/"+itemID.String(), "", userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
		"item_id":  itemID.String(),
	})

	if err := h.DeleteBlockExercise(c); err != nil {
		t.Fatalf("DeleteBlockExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteBlockExercise_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	store := &fakeProgramStore{program: detail.Program, err: ErrNotFound}
	h := programHandler(store)

	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/weeks/"+uuid.New().String()+"/blocks/"+uuid.New().String()+"/exercises/"+uuid.New().String(), "", userID, map[string]string{
		"id":       programID.String(),
		"week_id":  uuid.New().String(),
		"block_id": uuid.New().String(),
		"item_id":  uuid.New().String(),
	})

	err := h.DeleteBlockExercise(c)
	assertHTTPError(t, err, http.StatusNotFound)
}

type publishStore struct {
	fakeProgramStore
}

func (p *publishStore) PublishFromDraft(_ context.Context, _, _ uuid.UUID, d Detail) (Detail, error) {
	if p.err != nil {
		return Detail{}, p.err
	}
	d.Status = StatusPublished
	return d, nil
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
		roles *fakeRoleQuerier
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
				program: Program{ID: programID, CreatedBy: uuid.New()},
			},
			roles: &fakeRoleQuerier{isAdmin: false},
			code:  http.StatusForbidden,
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
			roles := tt.roles
			if roles == nil {
				roles = &fakeRoleQuerier{isAdmin: true}
			}
			h := programHandlerWithRoles(tt.store, roles)
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
			Weeks:   []Week{{WeekNumber: 1, Days: []Day{{DayNumber: 1, Blocks: []DayBlock{{BlockType: BlockTypeComplex}}}}}},
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

func TestHandlers_Delete_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/programs/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.Delete(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_Delete_invalidID(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/programs/not-a-uuid", "", uuid.New(), map[string]string{"id": "not-a-uuid"})
	err := h.Delete(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_Delete_notFound(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{err: ErrNotFound})
	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/programs/"+uuid.New().String(), "", userID, map[string]string{"id": uuid.New().String()})
	err := h.Delete(c)
	assertHTTPError(t, err, http.StatusNotFound)
}

func TestHandlers_AddWeek_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/programs/"+uuid.New().String()+"/weeks", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.AddWeek(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_Archive_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/programs/"+uuid.New().String()+"/archive", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.Archive(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_Publish_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/programs/"+uuid.New().String()+"/publish", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(uuid.New().String())
	err := h.Publish(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_AddDay_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/programs/"+uuid.New().String()+"/weeks/"+uuid.New().String()+"/days", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "week_id")
	c.SetParamValues(uuid.New().String(), uuid.New().String())
	err := h.AddDay(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}
