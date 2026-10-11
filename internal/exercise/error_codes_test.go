package exercise

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/subscription"
)

func TestQuotaExceeded_bodyHasAllSevenFields(t *testing.T) {
	qe := &subscription.QuotaError{Resource: subscription.ResourceExercises, Plan: subscription.PlanFree, Limit: 10, Usage: 10}
	store := &fakeExerciseStore{createErr: qe, updateErr: qe}
	h := testExerciseHandlers(store)
	asUser := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set(auth.ContextUserIDKey, uuid.New())
			return next(c)
		}
	}
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	e.POST("/exercises", h.Create, asUser)
	e.PUT("/exercises/:id", h.Update, asUser)

	want := map[string]any{
		"error": "quota_exceeded", "resource": "exercises", "plan": "free", "limit": 10.0, "usage": 10.0,
		"code": "quota_exceeded", "message": "quota exceeded",
	}
	for name, req := range map[string]*http.Request{
		"create": httptest.NewRequest(http.MethodPost, "/exercises", strings.NewReader(validUpsertJSON())),
		"update": httptest.NewRequest(http.MethodPut, "/exercises/"+uuid.NewString(), strings.NewReader(validUpsertJSON())),
	} {
		t.Run(name, func(t *testing.T) {
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body)
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("body is not JSON: %v; body = %s", err, rec.Body)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("body = %v, want %v", got, want)
			}
		})
	}
}
