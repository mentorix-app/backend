package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
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

func TestRegisterReady_redisOK(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	e := echo.New()
	RegisterReady(e, nil, rdb)

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
	if resp.Status != ReadyStatusReady {
		t.Fatalf("status = %q, want %q", resp.Status, ReadyStatusReady)
	}
	if resp.Checks["redis"] != StatusOK {
		t.Fatalf("redis check = %q", resp.Checks["redis"])
	}
}

func TestRegisterReady_redisUnavailable(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	e := echo.New()
	RegisterReady(e, nil, rdb)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var resp ReadyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != ReadyStatusNotReady || resp.Checks["redis"] != CheckError {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestNewRedisClient_empty(t *testing.T) {
	rdb, err := NewRedisClient("")
	if err != nil || rdb != nil {
		t.Fatalf("NewRedisClient(\"\") = %v, %v", rdb, err)
	}
}

func TestNewRedisClient_invalidURL(t *testing.T) {
	_, err := NewRedisClient("not-a-redis-url")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewPool_empty(t *testing.T) {
	pool, err := NewPool(context.Background(), "")
	if err != nil || pool != nil {
		t.Fatalf("NewPool(\"\") = %v, %v", pool, err)
	}
}

func TestNewPool_invalidURL(t *testing.T) {
	_, err := NewPool(context.Background(), "://not-postgres")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterReady_databaseUnavailable(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://mentorix:mentorix@127.0.0.1:1/mentorix?connect_timeout=1")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewWithConfig: %v", err)
	}
	defer pool.Close()

	e := echo.New()
	RegisterReady(e, pool, nil)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var resp ReadyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Checks["database"] != CheckError {
		t.Fatalf("database check = %q, want error", resp.Checks["database"])
	}
}
