package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRegisterLiveness(t *testing.T) {
	e := echo.New()
	RegisterLiveness(e)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != StatusOK {
		t.Fatalf("status = %q, want %q", body["status"], StatusOK)
	}
}

func TestRegisterReady_noDependencies(t *testing.T) {
	e := echo.New()
	RegisterReady(e, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp ReadyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != ReadyStatusNoDepsConfigured {
		t.Fatalf("status = %q, want %q", resp.Status, ReadyStatusNoDepsConfigured)
	}
	if resp.Checks["database"] != CheckSkipped || resp.Checks["redis"] != CheckSkipped {
		t.Fatalf("checks = %+v", resp.Checks)
	}
}
