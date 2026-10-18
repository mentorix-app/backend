package http

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
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

func TestConfigureIPExtractor_cloudflare(t *testing.T) {
	const defectXFF = "198.51.100.7, 172.71.1.1"

	tests := []struct {
		name       string
		trusted    []string
		remoteAddr string
		xff        string
		cfIP       string
		want       string
	}{
		{
			name:    "client behind a Cloudflare edge node",
			trusted: []string{"private", "cloudflare"},
			xff:     defectXFF,
			cfIP:    "198.51.100.7",
			want:    "198.51.100.7",
		},
		{
			name:    "forged leftmost XFF entry is ignored",
			trusted: []string{"private", "cloudflare"},
			xff:     "1.2.3.4, 198.51.100.7, 162.158.0.9",
			cfIP:    "198.51.100.7",
			want:    "198.51.100.7",
		},
		{
			name:    "forged CF header when the hop is not Cloudflare",
			trusted: []string{"private", "cloudflare"},
			xff:     "203.0.113.9",
			cfIP:    "198.51.100.1",
			want:    "203.0.113.9",
		},
		{
			name:    "garbage CF header falls back to the hop",
			trusted: []string{"private", "cloudflare"},
			xff:     defectXFF,
			cfIP:    "garbage",
			want:    "172.71.1.1",
		},
		{
			name:    "missing CF header falls back to the hop",
			trusted: []string{"private", "cloudflare"},
			xff:     defectXFF,
			cfIP:    "",
			want:    "172.71.1.1",
		},
		{
			name:    "CF header with several addresses falls back to the hop",
			trusted: []string{"private", "cloudflare"},
			xff:     defectXFF,
			cfIP:    "1.2.3.4, 5.6.7.8",
			want:    "172.71.1.1",
		},
		{
			name:    "IPv6 client is returned in canonical form",
			trusted: []string{"private", "cloudflare"},
			xff:     "2001:db8::5, 2606:4700::1",
			cfIP:    "2001:db8:0:0::5",
			want:    "2001:db8::5",
		},
		{
			name:    "Cloudflare Worker subrequest reports the Worker address",
			trusted: []string{"private", "cloudflare"},
			xff:     "198.51.100.66, 172.71.1.1",
			cfIP:    "2a06:98c0:3600::103",
			want:    "2a06:98c0:3600::103",
		},
		{
			name:    "IPv4-mapped hop and header are unmapped",
			trusted: []string{"private", "cloudflare"},
			xff:     "198.51.100.7, ::ffff:172.71.1.1",
			cfIP:    "::ffff:198.51.100.7",
			want:    "198.51.100.7",
		},
		{
			name:    "zone in the CF header never reaches the result",
			trusted: []string{"private", "cloudflare"},
			xff:     defectXFF,
			cfIP:    "2001:db8::5%eth0",
			want:    "2001:db8::5",
		},
		{
			name:    "keyword is case-insensitive",
			trusted: []string{"private", "Cloudflare"},
			xff:     defectXFF,
			cfIP:    "198.51.100.7",
			want:    "198.51.100.7",
		},
		{
			name:       "keyword alone reads the direct peer",
			trusted:    []string{"cloudflare"},
			remoteAddr: "172.71.1.1:443",
			cfIP:       "198.51.100.7",
			want:       "198.51.100.7",
		},
		{
			name:    "without the keyword the hop is returned",
			trusted: []string{"private"},
			xff:     defectXFF,
			cfIP:    "198.51.100.7",
			want:    "172.71.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			ConfigureIPExtractor(e, tt.trusted)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "10.0.0.1:1234"
			if tt.remoteAddr != "" {
				req.RemoteAddr = tt.remoteAddr
			}
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.cfIP != "" {
				req.Header.Set("CF-Connecting-IP", tt.cfIP)
			}
			if got := e.IPExtractor(req); got != tt.want {
				t.Errorf("IP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInCloudflare_boundaries(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"162.159.255.255", true},
		{"162.160.0.0", false},
		{"104.23.255.255", true},
		{"104.24.0.0", true},
		{"104.28.0.0", false},
		{"2a06:98c7:ffff::1", true},
		{"2a06:98c8::1", false},
		{"172.63.255.255", false},
		{"172.64.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := inCloudflare(netip.MustParseAddr(tt.addr)); got != tt.want {
				t.Errorf("inCloudflare(%s) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}
