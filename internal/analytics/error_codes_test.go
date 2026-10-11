package analytics_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/analytics"
	httpx "mentorix-backend/internal/http"
)

// "client not found" is the text trainerclient.HTTPErrorFrom answers with the
// code client_not_found, so the analytics answer carries the same code.
func TestHTTPErrorFrom_codes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"client not found", analytics.ErrClientNotFound, 404, "client_not_found", "client not found"},
		{"program not found keeps the default", analytics.ErrProgramNotFound, 404, "not_found", httpx.MsgProgramNotFound},
		{"forbidden keeps the default", analytics.ErrForbidden, 403, "forbidden", httpx.MsgForbidden},
		{"validation keeps the default", analytics.ErrValidation, 400, "bad_request", analytics.ErrValidation.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			e.HTTPErrorHandler = httpx.ErrorHandler
			e.GET("/x", func(echo.Context) error { return analytics.HTTPErrorFrom(tt.err) })
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body)
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
			}
			want := map[string]any{"code": tt.wantCode, "message": tt.wantMessage}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("body = %v, want %v", got, want)
			}
		})
	}
}
