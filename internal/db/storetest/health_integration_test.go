//go:build integration

package storetest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"mentorix-backend/internal/health"
)

func TestHealth_NewPool(t *testing.T) {
	_, connStr := NewPoolWithURL(t)

	pool, err := health.NewPool(context.Background(), connStr)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestHealth_RegisterReady_allDependenciesOK(t *testing.T) {
	pool, _ := NewPoolWithURL(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	e := echo.New()
	health.RegisterReady(e, pool, rdb)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp health.ReadyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != health.ReadyStatusReady {
		t.Fatalf("status = %q, want %q", resp.Status, health.ReadyStatusReady)
	}
	if resp.Checks["database"] != health.StatusOK || resp.Checks["redis"] != health.StatusOK {
		t.Fatalf("checks = %+v", resp.Checks)
	}
}
