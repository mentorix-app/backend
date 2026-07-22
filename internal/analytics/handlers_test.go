package analytics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/analytics"
	"mentorix-backend/internal/auth"
)

func testHandlers() *analytics.Handlers {
	svc := analytics.NewService(nil, "test-jwt-secret-at-least-32-chars-long")
	return analytics.NewHandlers(svc, nil, "test-jwt-secret-at-least-32-chars-long")
}

func testContext(e *echo.Echo, target string, userID uuid.UUID, params map[string]string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if userID != uuid.Nil {
		c.Set(auth.ContextUserIDKey, userID)
	}
	for k, v := range params {
		c.SetParamNames(k)
		c.SetParamValues(v)
	}
	return c, rec
}

func TestHandlers_Mount_registersRoutes(t *testing.T) {
	h := testHandlers()
	e := echo.New()
	h.Mount(e)

	found := map[string]bool{}
	for _, r := range e.Routes() {
		found[r.Method+" "+r.Path] = true
	}
	want := []string{
		"GET /trainer/clients/:client_user_id/analytics",
		"GET /trainer/clients/:client_user_id/completions",
		"GET /trainer/programs/analytics",
		"GET /trainer/programs/:program_id/analytics",
		"GET /trainer/programs/:program_id/weeks/:week_number/results",
	}
	for _, rt := range want {
		if !found[rt] {
			t.Fatalf("route %s missing; got %v", rt, found)
		}
	}
}

func TestHandlers_unauthorized(t *testing.T) {
	h := testHandlers()
	e := echo.New()

	checks := []func(echo.Context) error{
		h.GetClientAnalytics,
		h.ListClientCompletions,
		h.ListProgramsAnalytics,
		h.GetProgramAnalytics,
		h.GetProgramWeekResults,
	}
	for i, fn := range checks {
		c, _ := testContext(e, "/", uuid.Nil, nil)
		err := fn(c)
		he, ok := err.(*echo.HTTPError)
		if !ok || he.Code != http.StatusUnauthorized {
			t.Fatalf("handler %d: err = %v, want 401", i, err)
		}
	}
}

func TestHandlers_invalidID(t *testing.T) {
	h := testHandlers()
	e := echo.New()
	userID := uuid.New()

	tests := []struct {
		name  string
		fn    func(echo.Context) error
		param string
	}{
		{"client analytics", h.GetClientAnalytics, "client_user_id"},
		{"client completions", h.ListClientCompletions, "client_user_id"},
		{"program analytics", h.GetProgramAnalytics, "program_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := testContext(e, "/", userID, map[string]string{tt.param: "not-a-uuid"})
			err := tt.fn(c)
			he, ok := err.(*echo.HTTPError)
			if !ok || he.Code != http.StatusBadRequest {
				t.Fatalf("err = %v, want 400", err)
			}
		})
	}
}

func TestHandlers_invalidQueryParams(t *testing.T) {
	h := testHandlers()
	e := echo.New()
	userID := uuid.New()
	clientID := uuid.New()

	c, _ := testContext(e, "/?page=0", userID, map[string]string{"client_user_id": clientID.String()})
	if err := h.ListClientCompletions(c); err == nil {
		t.Fatal("want 400 for page=0")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("err = %v, want 400", err)
	}

	c, _ = testContext(e, "/?sort_by=bogus", userID, nil)
	if err := h.ListProgramsAnalytics(c); err == nil {
		t.Fatal("want 400 for sort_by=bogus")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("err = %v, want 400", err)
	}
}

func TestHandlers_invalidWeekNumber(t *testing.T) {
	h := testHandlers()
	e := echo.New()
	userID := uuid.New()
	programID := uuid.New()

	c, _ := testContext(e, "/", userID, map[string]string{
		"program_id":  programID.String(),
		"week_number": "not-a-number",
	})
	// Set both params explicitly (testContext only sets one name/value pair at a time when looping).
	c.SetParamNames("program_id", "week_number")
	c.SetParamValues(programID.String(), "not-a-number")
	err := h.GetProgramWeekResults(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("err = %v, want 400", err)
	}
}

func TestHTTPErrorFrom(t *testing.T) {
	tests := []struct {
		err  error
		code int
	}{
		{analytics.ErrValidation, http.StatusBadRequest},
		{analytics.ErrForbidden, http.StatusForbidden},
		{analytics.ErrClientNotFound, http.StatusNotFound},
		{analytics.ErrProgramNotFound, http.StatusNotFound},
		{analytics.ErrWeekNotFound, http.StatusNotFound},
		{echo.ErrTeapot, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		if got := analytics.HTTPErrorFrom(tt.err); got.Code != tt.code {
			t.Errorf("HTTPErrorFrom(%v) = %d, want %d", tt.err, got.Code, tt.code)
		}
	}
}
