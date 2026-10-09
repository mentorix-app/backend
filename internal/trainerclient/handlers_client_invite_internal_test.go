package trainerclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
)

const clientInviteTestSecret = "test-jwt-secret-at-least-32-chars-long"

type fakeInviteAcceptor struct {
	result AcceptInviteResult
	err    error
	called int
	userID uuid.UUID
	token  string
}

func (f *fakeInviteAcceptor) AcceptInviteAsUser(_ context.Context, userID uuid.UUID, rawToken string) (AcceptInviteResult, error) {
	f.called++
	f.userID, f.token = userID, rawToken
	return f.result, f.err
}

func clientInviteEcho(f *fakeInviteAcceptor) *echo.Echo {
	h := NewHandlers(nil, nil, clientInviteTestSecret)
	h.invites = f
	e := echo.New()
	h.Mount(e)
	return e
}

func bearerFor(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Issuer:    auth.Issuer,
		Subject:   userID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(clientInviteTestSecret))
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + s
}

func postAccept(e *echo.Echo, authz, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/client/invites/accept", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if authz != "" {
		req.Header.Set(echo.HeaderAuthorization, authz)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestClientAcceptInvite_success(t *testing.T) {
	userID, trainerID := uuid.New(), uuid.New()
	f := &fakeInviteAcceptor{result: AcceptInviteResult{
		UserID: userID, TrainerID: trainerID, TrainerDisplayName: "Anna", AlreadyLinked: true,
	}}
	rec := postAccept(clientInviteEcho(f), bearerFor(t, userID), `{"token":"inv_abc"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.userID != userID || f.token != "inv_abc" {
		t.Fatalf("service got %v, %q", f.userID, f.token)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"trainer_id": trainerID.String(), "trainer_display_name": "Anna", "already_linked": true}
	if len(raw) != len(want) {
		t.Fatalf("body = %v, want exactly %v", raw, want)
	}
	for k, v := range want {
		if raw[k] != v {
			t.Errorf("%s = %v, want %v", k, raw[k], v)
		}
	}
}

func TestClientAcceptInvite_requiresToken(t *testing.T) {
	f := &fakeInviteAcceptor{}
	e := clientInviteEcho(f)
	for name, authz := range map[string]string{"missing": "", "malformed": "Bearer nope"} {
		rec := postAccept(e, authz, `{"token":"abc"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
		}
	}
	if f.called != 0 {
		t.Fatalf("service called %d times without a valid token", f.called)
	}
}

func TestClientAcceptInvite_invalidJSON(t *testing.T) {
	f := &fakeInviteAcceptor{}
	rec := postAccept(clientInviteEcho(f), bearerFor(t, uuid.New()), `not-json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if f.called != 0 {
		t.Fatal("service must not be called")
	}
}

func TestClientAcceptInvite_errorStatuses(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    int
		message string
	}{
		{"invite not found", ErrInviteNotFound, http.StatusNotFound, ""},
		{"user not found", ErrUserNotFound, http.StatusNotFound, ""},
		{"consumed", ErrInviteConsumed, http.StatusConflict, "invite already consumed"},
		{"client limit", ErrClientLimitReached, http.StatusConflict, "client limit reached"},
		{"expired", ErrInviteExpired, http.StatusGone, ""},
		{"admin caller", ErrAdminCannotAccept, http.StatusForbidden, ""},
		{"own invite", ErrSelfInvite, http.StatusUnprocessableEntity, ""},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeInviteAcceptor{err: tt.err}
			rec := postAccept(clientInviteEcho(f), bearerFor(t, uuid.New()), `{"token":"abc"}`)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tt.want, rec.Body)
			}
			if tt.message == "" {
				return
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body["message"] != tt.message {
				t.Fatalf("body = %s, want exactly message %q", rec.Body, tt.message)
			}
		})
	}
}

// Per-route middleware: a /client group with JWT would guard the public signed-link
// analytics route and turn unknown paths into 401.
func TestClientInviteMount_doesNotGuardOtherClientPaths(t *testing.T) {
	h := NewHandlers(nil, nil, clientInviteTestSecret)
	h.invites = &fakeInviteAcceptor{}
	e := echo.New()
	e.GET("/client/analytics", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	h.Mount(e)

	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/client/analytics", http.StatusOK},
		{http.MethodGet, "/client/unknown", http.StatusNotFound},
		{http.MethodGet, "/client/invites/accept", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.want {
			t.Errorf("%s %s = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}
