package http

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRequestLog_success(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	e := echo.New()
	e.Use(RequestLog(logger))
	e.GET("/ok", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	log := buf.String()
	if !strings.Contains(log, "level=INFO") {
		t.Fatalf("want INFO log, got %q", log)
	}
	if !strings.Contains(log, "status=200") {
		t.Fatalf("want status=200, got %q", log)
	}
}

func TestRequestLog_httpErrorUsesRealStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	e := echo.New()
	e.Use(RequestLog(logger))
	e.GET("/missing", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusNotFound, "avatar not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	log := buf.String()
	if !strings.Contains(log, "level=WARN") {
		t.Fatalf("want WARN for 4xx, got %q", log)
	}
	if !strings.Contains(log, "status=404") {
		t.Fatalf("want status=404, got %q", log)
	}
	if !strings.Contains(log, "avatar not found") {
		t.Fatalf("want error message, got %q", log)
	}
}

func TestRequestLog_internalError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	e := echo.New()
	e.Use(RequestLog(logger))
	e.GET("/fail", func(c echo.Context) error {
		return errors.New("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	log := buf.String()
	if !strings.Contains(log, "level=ERROR") {
		t.Fatalf("want ERROR for 5xx, got %q", log)
	}
	if !strings.Contains(log, "status=500") {
		t.Fatalf("want status=500, got %q", log)
	}
	if !strings.Contains(log, "boom") {
		t.Fatalf("log = %q", log)
	}
}

func TestConfigureIPExtractor_direct(t *testing.T) {
	e := echo.New()
	ConfigureIPExtractor(e, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	if got := e.IPExtractor(req); got != "203.0.113.10" {
		t.Errorf("IP = %q, want 203.0.113.10", got)
	}
}

func TestConfigureIPExtractor_trustedProxy(t *testing.T) {
	e := echo.New()
	ConfigureIPExtractor(e, []string{"private", "loopback", "203.0.113.0/24", "10.0.0.5"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.20")
	if got := e.IPExtractor(req); got != "203.0.113.20" {
		t.Errorf("IP = %q, want 203.0.113.20", got)
	}
}

func TestConfigureIPExtractor_invalidEntries(t *testing.T) {
	e := echo.New()
	ConfigureIPExtractor(e, []string{"", "not-a-cidr"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.1:9999"
	if got := e.IPExtractor(req); got != "198.51.100.1" {
		t.Errorf("IP = %q, want direct IP", got)
	}
}
