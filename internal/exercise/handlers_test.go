package exercise

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type fakeExerciseStore struct {
	deleteManyCount int64
	deleteManyErr   error
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

func (f *fakeExerciseStore) DeleteMany(context.Context, []uuid.UUID) (int64, error) {
	if f.deleteManyErr != nil {
		return 0, f.deleteManyErr
	}
	return f.deleteManyCount, nil
}

func testDeleteManyHandlers(store *fakeExerciseStore) *Handlers {
	return &Handlers{svc: &Service{store: store}}
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
			h := testDeleteManyHandlers(tt.store)
			e.DELETE("/exercises", h.DeleteMany)

			req := httptest.NewRequest(http.MethodDelete, "/exercises", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

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
