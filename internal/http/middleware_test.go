package http

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	if !bytes.Contains(buf.Bytes(), []byte("request")) {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRequestLog_error(t *testing.T) {
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

	if !bytes.Contains(buf.Bytes(), []byte("boom")) {
		t.Fatalf("log = %q", buf.String())
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
