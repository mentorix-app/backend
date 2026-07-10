package trainerclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/program"
)

type stubClientPrograms struct {
	assignment       *program.Assignment
	assignmentErr    error
	setAssignment    *program.Assignment
	setAssignmentErr error
	bulkResult       program.BulkAssignmentResult
	bulkResultErr    error
}

func (s stubClientPrograms) GetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID) (*program.Assignment, error) {
	return s.assignment, s.assignmentErr
}

func (s stubClientPrograms) BulkSetClientProgramAssignment(context.Context, uuid.UUID, program.BulkSetClientProgramAssignmentRequest) (program.BulkAssignmentResult, error) {
	if s.bulkResultErr != nil {
		return program.BulkAssignmentResult{}, s.bulkResultErr
	}
	if s.bulkResult.Assigned != nil || s.bulkResult.Cleared != nil || s.bulkResult.Skipped != nil {
		return s.bulkResult, nil
	}
	if s.setAssignmentErr != nil {
		return program.BulkAssignmentResult{}, s.setAssignmentErr
	}
	if s.setAssignment != nil {
		return program.BulkAssignmentResult{Assigned: []program.Assignment{*s.setAssignment}}, nil
	}
	return program.BulkAssignmentResult{Cleared: []uuid.UUID{}}, nil
}

func trainerClientContext(e *echo.Echo, method, path, body string, userID uuid.UUID, params map[string]string) (echo.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(path)
	if len(params) > 0 {
		names := make([]string, 0, len(params))
		values := make([]string, 0, len(params))
		for k, v := range params {
			names = append(names, k)
			values = append(values, v)
		}
		c.SetParamNames(names...)
		c.SetParamValues(values...)
	}
	c.SetRequest(req.WithContext(req.Context()))
	c.Set(auth.ContextUserIDKey, userID)
	return c, rec
}

func TestHandlers_GetProgramAssignment_unauthorized(t *testing.T) {
	h := testHandlers(stubClientPrograms{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/trainer/clients/"+uuid.New().String()+"/program-assignment", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("client_user_id")
	c.SetParamValues(uuid.New().String())

	err := h.GetProgramAssignment(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}

func TestHandlers_SetProgramAssignment_unauthorized(t *testing.T) {
	h := testHandlers(stubClientPrograms{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/trainer/clients/program-assignment", strings.NewReader(`{"client_user_ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.SetProgramAssignment(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}

func TestHandlers_SetProgramAssignment_invalidJSON(t *testing.T) {
	userID := uuid.New()
	h := testHandlers(stubClientPrograms{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/trainer/clients/program-assignment", strings.NewReader(`not-json`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserIDKey, userID)

	err := h.SetProgramAssignment(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestHandlers_GetProgramAssignment_nilAssignment(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	h := testHandlers(stubClientPrograms{
		assignment: nil,
	})

	e := echo.New()
	c, rec := trainerClientContext(e, http.MethodGet, "/trainer/clients/"+clientID.String()+"/program-assignment", "", userID, map[string]string{
		"client_user_id": clientID.String(),
	})

	if err := h.GetProgramAssignment(c); err != nil {
		t.Fatalf("GetProgramAssignment: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "null" {
		t.Fatalf("body = %q, want null", body)
	}
}

func TestHandlers_GetProgramAssignment_success(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	now := time.Now().UTC()
	h := testHandlers(stubClientPrograms{
		assignment: &program.Assignment{
			ID:           uuid.New(),
			ClientUserID: clientID,
			AssignedAt:   now,
			CreatedAt:    now,
			Status:       program.AssignmentStatusActive,
		},
	})

	e := echo.New()
	c, rec := trainerClientContext(e, http.MethodGet, "/trainer/clients/"+clientID.String()+"/program-assignment", "", userID, map[string]string{
		"client_user_id": clientID.String(),
	})

	if err := h.GetProgramAssignment(c); err != nil {
		t.Fatalf("GetProgramAssignment: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlers_SetProgramAssignment_clear(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	h := testHandlers(stubClientPrograms{
		bulkResult: program.BulkAssignmentResult{Cleared: []uuid.UUID{clientID}},
	})

	e := echo.New()
	body := `{"program_id":null,"client_user_ids":["` + clientID.String() + `"]}`
	c, rec := trainerClientContext(e, http.MethodPut, "/trainer/clients/program-assignment", body, userID, nil)

	if err := h.SetProgramAssignment(c); err != nil {
		t.Fatalf("SetProgramAssignment: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlers_SetProgramAssignment_success(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	programID := uuid.New()
	now := time.Now().UTC()
	h := testHandlers(stubClientPrograms{
		bulkResult: program.BulkAssignmentResult{
			Assigned: []program.Assignment{{
				ID:           uuid.New(),
				ProgramID:    programID,
				ClientUserID: clientID,
				AssignedAt:   now,
				CreatedAt:    now,
				Status:       program.AssignmentStatusActive,
			}},
		},
	})

	e := echo.New()
	body := `{"program_id":"` + programID.String() + `","client_user_ids":["` + clientID.String() + `"]}`
	c, rec := trainerClientContext(e, http.MethodPut, "/trainer/clients/program-assignment", body, userID, nil)

	if err := h.SetProgramAssignment(c); err != nil {
		t.Fatalf("SetProgramAssignment: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlers_SetProgramAssignment_validationError(t *testing.T) {
	h := testHandlers(stubClientPrograms{
		bulkResultErr: fmt.Errorf("%w: client_user_ids is required", program.ErrValidation),
	})
	e := echo.New()
	c, _ := trainerClientContext(e, http.MethodPut, "/trainer/clients/program-assignment", `{"client_user_ids":[]}`, uuid.New(), nil)

	err := h.SetProgramAssignment(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}

func TestHandlers_SetProgramAssignment_programError(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	programID := uuid.New()
	h := testHandlers(stubClientPrograms{
		bulkResultErr: program.ErrProgramNotPublished,
	})

	e := echo.New()
	body := `{"program_id":"` + programID.String() + `","client_user_ids":["` + clientID.String() + `"]}`
	c, _ := trainerClientContext(e, http.MethodPut, "/trainer/clients/program-assignment", body, userID, nil)

	err := h.SetProgramAssignment(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnprocessableEntity {
		t.Fatalf("error = %v, want 422", err)
	}
}

func TestHandlers_GetProgramAssignment_invalidClientID(t *testing.T) {
	h := testHandlers(stubClientPrograms{})
	e := echo.New()
	c, _ := trainerClientContext(e, http.MethodGet, "/trainer/clients/not-a-uuid/program-assignment", "", uuid.New(), map[string]string{
		"client_user_id": "not-a-uuid",
	})

	err := h.GetProgramAssignment(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("error = %v, want 400", err)
	}
}
