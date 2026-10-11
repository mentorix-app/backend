package subscription

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
)

func TestQuotaHTTPError_bodyHasAllSevenFields(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	e.GET("/x", func(echo.Context) error {
		return QuotaHTTPError(&QuotaError{Resource: ResourcePrograms, Plan: PlanFree, Limit: 3, Usage: 3})
	})
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
	}
	want := map[string]any{
		"error":    "quota_exceeded",
		"resource": "programs",
		"plan":     "free",
		"limit":    3.0,
		"usage":    3.0,
		"code":     "quota_exceeded",
		"message":  "quota exceeded",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}
