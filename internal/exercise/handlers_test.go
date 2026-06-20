package exercise

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type fakeExerciseStore struct {
	deleteAllCount int64
	deleteAllErr   error
}

func (f *fakeExerciseStore) List(context.Context, ListParams) (ListResult, error) {
	panic("not implemented")
}

func (f *fakeExerciseStore) GetByID(context.Context, uuid.UUID) (Exercise, error) {
	panic("not implemented")
}

func (f *fakeExerciseStore) Create(context.Context, uuid.UUID, UpsertInput) (Exercise, error) {
	panic("not implemented")
}

func (f *fakeExerciseStore) Update(context.Context, uuid.UUID, uuid.UUID, UpsertInput) (Exercise, error) {
	panic("not implemented")
}

func (f *fakeExerciseStore) Delete(context.Context, uuid.UUID) error {
	panic("not implemented")
}

func (f *fakeExerciseStore) DeleteAll(context.Context) (int64, error) {
	if f.deleteAllErr != nil {
		return 0, f.deleteAllErr
	}
	return f.deleteAllCount, nil
}

func testDeleteAllHandlers(store *fakeExerciseStore) *Handlers {
	return &Handlers{svc: &Service{store: store}}
}

func TestDeleteAll_success(t *testing.T) {
	e := echo.New()
	h := testDeleteAllHandlers(&fakeExerciseStore{deleteAllCount: 3})
	e.DELETE("/exercises/all", h.DeleteAll)

	req := httptest.NewRequest(http.MethodDelete, "/exercises/all", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body deleteAllResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.DeletedCount != 3 {
		t.Fatalf("deleted_count = %d, want 3", body.DeletedCount)
	}
}

func TestDeleteAll_serviceError(t *testing.T) {
	e := echo.New()
	h := testDeleteAllHandlers(&fakeExerciseStore{deleteAllErr: errors.New("db down")})
	e.DELETE("/exercises/all", h.DeleteAll)

	req := httptest.NewRequest(http.MethodDelete, "/exercises/all", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
