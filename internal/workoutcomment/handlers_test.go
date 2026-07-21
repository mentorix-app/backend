package workoutcomment

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
)

func commentContext(e *echo.Echo, body string, userID uuid.UUID, clientUserID, completionID string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if userID != uuid.Nil {
		c.Set(auth.ContextUserIDKey, userID)
	}
	c.SetParamNames("client_user_id", "completion_id")
	c.SetParamValues(clientUserID, completionID)
	return c, rec
}

func TestHandlers_Mount_registersRoute(t *testing.T) {
	h := NewHandlers(NewServiceWithStore(&fakeStore{}), nil, "test-jwt-secret-at-least-32-chars-long")
	e := echo.New()
	h.Mount(e)

	want := "POST /trainer/clients/:client_user_id/completions/:completion_id/comments"
	for _, r := range e.Routes() {
		if r.Method+" "+r.Path == want {
			return
		}
	}
	t.Fatalf("route %s missing", want)
}

func TestHandlers_CreateComment_unauthorized(t *testing.T) {
	h := NewHandlers(NewServiceWithStore(&fakeStore{}), nil, "secret")
	e := echo.New()
	c, _ := commentContext(e, `{"text":"ok"}`, uuid.Nil, uuid.NewString(), uuid.NewString())

	err := h.CreateComment(c)
	assertHTTPError(t, err, http.StatusUnauthorized)
}

func TestHandlers_CreateComment_invalidIDs(t *testing.T) {
	h := NewHandlers(NewServiceWithStore(&fakeStore{}), nil, "secret")
	e := echo.New()

	tests := []struct {
		name                     string
		clientUserID, completion string
	}{
		{"bad client id", "not-a-uuid", uuid.NewString()},
		{"bad completion id", uuid.NewString(), "not-a-uuid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := commentContext(e, `{"text":"ok"}`, uuid.New(), tt.clientUserID, tt.completion)
			err := h.CreateComment(c)
			assertHTTPError(t, err, http.StatusBadRequest)
		})
	}
}

func TestHandlers_CreateComment_invalidJSON(t *testing.T) {
	h := NewHandlers(NewServiceWithStore(&fakeStore{}), nil, "secret")
	e := echo.New()
	c, _ := commentContext(e, `{"text":`, uuid.New(), uuid.NewString(), uuid.NewString())

	err := h.CreateComment(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

func TestHandlers_CreateComment_created(t *testing.T) {
	store := &fakeStore{comment: Comment{ID: uuid.New(), Text: "ok", CreatedAt: time.Now().UTC()}}
	h := NewHandlers(NewServiceWithStore(store), nil, "secret")
	e := echo.New()
	c, rec := commentContext(e, `{"text":"ok"}`, uuid.New(), uuid.NewString(), uuid.NewString())

	if err := h.CreateComment(c); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	body := rec.Body.String()
	for _, key := range []string{`"id"`, `"text"`, `"created_at"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("body %s missing %s", body, key)
		}
	}
}

func TestHTTPErrorFrom_mapping(t *testing.T) {
	tests := []struct {
		err  error
		code int
	}{
		{ErrValidation, http.StatusBadRequest},
		{ErrForbidden, http.StatusForbidden},
		{ErrCompletionNotFound, http.StatusNotFound},
		{ErrCommentExists, http.StatusConflict},
		{echo.ErrTeapot, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		if got := HTTPErrorFrom(tt.err); got.Code != tt.code {
			t.Errorf("HTTPErrorFrom(%v) = %d, want %d", tt.err, got.Code, tt.code)
		}
	}
}

func assertHTTPError(t *testing.T, err error, code int) {
	t.Helper()
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("error = %v, want *echo.HTTPError", err)
	}
	if he.Code != code {
		t.Fatalf("code = %d, want %d", he.Code, code)
	}
}
