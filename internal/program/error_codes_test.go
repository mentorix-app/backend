package program

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/subscription"
)

func renderMapProgramError(err error) *httptest.ResponseRecorder {
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	e.GET("/x", func(echo.Context) error { return HTTPErrorFrom(err) })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	return rec
}

func decodeObject(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
	}
	return got
}

// One text, one code: the sentinels that trainerclient.HTTPErrorFrom gives a
// specific code get the same code here wherever the text is the same.
func TestHTTPErrorFrom_codes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"client not linked", ErrClientNotLinked, 403, "client_not_linked", "client is not linked to trainer"},
		{"client blocked", ErrClientBlocked, 422, "client_blocked", "client is blocked"},
		{"program not published", ErrProgramNotPublished, 422, "program_not_published", "program is not published"},
		{"client not found is answered as user not found", ErrClientNotFound, 404, "user_not_found", httpx.MsgUserNotFound},
		{"wrapped sentinel keeps its code", fmt.Errorf("assign: %w", ErrClientBlocked), 422, "client_blocked", "assign: client is blocked"},
		{"program not found keeps the default", ErrNotFound, 404, "not_found", httpx.MsgProgramNotFound},
		{"forbidden keeps the default", ErrForbidden, 403, "forbidden", httpx.MsgForbidden},
		{"read only keeps the default", ErrReadOnly, 403, "forbidden", ErrReadOnly.Error()},
		{"invalid status transition keeps the default", ErrInvalidStatusTransition, 409, "conflict", ErrInvalidStatusTransition.Error()},
		{"unexpected keeps the default", errors.New("boom"), 500, "internal_error", "program operation failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := renderMapProgramError(tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body)
			}
			want := map[string]any{"code": tt.wantCode, "message": tt.wantMessage}
			if got := decodeObject(t, rec); !reflect.DeepEqual(got, want) {
				t.Fatalf("body = %v, want %v", got, want)
			}
		})
	}
}

func TestHTTPErrorFrom_quotaBodyHasAllSevenFields(t *testing.T) {
	qe := &subscription.QuotaError{Resource: subscription.ResourcePrograms, Plan: subscription.PlanFree, Limit: 3, Usage: 3}

	rec := renderMapProgramError(qe)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body)
	}
	want := map[string]any{
		"error": "quota_exceeded", "resource": "programs", "plan": "free", "limit": 3.0, "usage": 3.0,
		"code": "quota_exceeded", "message": "quota exceeded",
	}
	if got := decodeObject(t, rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}
