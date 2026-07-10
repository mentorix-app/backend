package trainerclient_test

import (
	"context"
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
}

func (s stubClientPrograms) GetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID) (*program.Assignment, error) {
	return s.assignment, s.assignmentErr
}

func (s stubClientPrograms) SetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (*program.Assignment, error) {
	return s.setAssignment, s.setAssignmentErr
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
	req := httptest.NewRequest(http.MethodPut, "/trainer/clients/"+uuid.New().String()+"/program-assignment", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("client_user_id")
	c.SetParamValues(uuid.New().String())

	err := h.SetProgramAssignment(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("error = %v, want 401", err)
	}
}

func TestHandlers_SetProgramAssignment_invalidJSON(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	h := testHandlers(stubClientPrograms{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/trainer/clients/"+clientID.String()+"/program-assignment", strings.NewReader(`not-json`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("client_user_id")
	c.SetParamValues(clientID.String())
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
		setAssignment: nil,
	})

	e := echo.New()
	c, rec := trainerClientContext(e, http.MethodPut, "/trainer/clients/"+clientID.String()+"/program-assignment", `{"program_id":null}`, userID, map[string]string{
		"client_user_id": clientID.String(),
	})

	if err := h.SetProgramAssignment(c); err != nil {
		t.Fatalf("SetProgramAssignment: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "null" {
		t.Fatalf("body = %q, want null", body)
	}
}

func TestHandlers_SetProgramAssignment_success(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	programID := uuid.New()
	now := time.Now().UTC()
	h := testHandlers(stubClientPrograms{
		setAssignment: &program.Assignment{
			ID:         uuid.New(),
			ProgramID:  programID,
			AssignedAt: now,
			CreatedAt:  now,
			Status:     program.AssignmentStatusActive,
		},
	})

	e := echo.New()
	body := `{"program_id":"` + programID.String() + `"}`
	c, rec := trainerClientContext(e, http.MethodPut, "/trainer/clients/"+clientID.String()+"/program-assignment", body, userID, map[string]string{
		"client_user_id": clientID.String(),
	})

	if err := h.SetProgramAssignment(c); err != nil {
		t.Fatalf("SetProgramAssignment: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlers_SetProgramAssignment_forbidden(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	programID := uuid.New()
	h := testHandlers(stubClientPrograms{
		setAssignmentErr: program.ErrClientNotLinked,
	})

	e := echo.New()
	body := `{"program_id":"` + programID.String() + `"}`
	c, _ := trainerClientContext(e, http.MethodPut, "/trainer/clients/"+clientID.String()+"/program-assignment", body, userID, map[string]string{
		"client_user_id": clientID.String(),
	})

	err := h.SetProgramAssignment(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusForbidden {
		t.Fatalf("error = %v, want 403", err)
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
