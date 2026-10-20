package http

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	testBFFSecret = "bff-shared-secret-0123456789-abcdef"
	testPeerIP    = "203.0.113.10"
)

func newBFFEcho(secret string, logger *slog.Logger, extra ...echo.MiddlewareFunc) *echo.Echo {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()
	e.Use(extra...)
	e.Use(BFFClientIP(secret, logger))
	e.GET("/ip", func(c echo.Context) error {
		return c.String(http.StatusOK, ClientIP(c))
	})
	return e
}

func bffRequest(secretHeader, clientIP string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = testPeerIP + ":4321"
	if secretHeader != "" {
		req.Header.Set(HeaderBFFSecret, secretHeader)
	}
	if clientIP != "" {
		req.Header.Set(HeaderBFFClientIP, clientIP)
	}
	return req
}

func TestBFFClientIP_selectsAddress(t *testing.T) {
	tests := []struct {
		name         string
		configured   string
		sentSecret   string
		sentClientIP string
		want         string
	}{
		{"valid secret and IPv4", testBFFSecret, testBFFSecret, "198.51.100.7", "198.51.100.7"},
		{"valid secret and IPv6", testBFFSecret, testBFFSecret, "2001:db8:0:0::5", "2001:db8::5"},
		{"IPv4-mapped IPv6 is unmapped", testBFFSecret, testBFFSecret, "::ffff:198.51.100.7", "198.51.100.7"},
		{"zone is dropped", testBFFSecret, testBFFSecret, "fe80::1%eth0", "fe80::1"},
		{"surrounding spaces are trimmed", testBFFSecret, testBFFSecret, "  198.51.100.7  ", "198.51.100.7"},
		{"wrong secret falls back", testBFFSecret, "another-secret-0123456789-abcdefgh", "198.51.100.7", testPeerIP},
		{"missing secret header falls back", testBFFSecret, "", "198.51.100.7", testPeerIP},
		{"secret of another length falls back", testBFFSecret, testBFFSecret + "x", "198.51.100.7", testPeerIP},
		{"prefix of the secret falls back", testBFFSecret, testBFFSecret[:10], "198.51.100.7", testPeerIP},
		{"empty configured secret disables the feature", "", "", "198.51.100.7", testPeerIP},
		{"empty configured secret ignores a secret header", "", testBFFSecret, "198.51.100.7", testPeerIP},
		{"not an address falls back", testBFFSecret, testBFFSecret, "not-an-ip", testPeerIP},
		{"several addresses fall back", testBFFSecret, testBFFSecret, "1.2.3.4, 5.6.7.8", testPeerIP},
		{"unspecified IPv4 falls back", testBFFSecret, testBFFSecret, "0.0.0.0", testPeerIP},
		{"unspecified IPv6 falls back", testBFFSecret, testBFFSecret, "::", testPeerIP},
		{"multicast IPv4 falls back", testBFFSecret, testBFFSecret, "224.0.0.1", testPeerIP},
		{"multicast IPv6 falls back", testBFFSecret, testBFFSecret, "ff02::1", testPeerIP},
		{"loopback is accepted", testBFFSecret, testBFFSecret, "127.0.0.1", "127.0.0.1"},
		{"private address is accepted", testBFFSecret, testBFFSecret, "10.1.2.3", "10.1.2.3"},
		{"missing client IP header falls back", testBFFSecret, testBFFSecret, "", testPeerIP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newBFFEcho(tt.configured, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, bffRequest(tt.sentSecret, tt.sentClientIP))
			if got := rec.Body.String(); got != tt.want {
				t.Errorf("ClientIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBFFClientIP_nilLoggerDoesNotPanic(t *testing.T) {
	e := newBFFEcho(testBFFSecret, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, bffRequest("wrong", "198.51.100.7"))
	if got := rec.Body.String(); got != testPeerIP {
		t.Errorf("ClientIP = %q, want %q", got, testPeerIP)
	}
}

func TestBFFClientIP_warnings(t *testing.T) {
	const forwarded = "198.51.100.7"
	const sentSecret = "another-secret-0123456789-abcdefgh"

	tests := []struct {
		name         string
		configured   string
		sentSecret   string
		sentClientIP string
		wantReason   string
	}{
		{"wrong secret", testBFFSecret, sentSecret, forwarded, "secret_mismatch"},
		{"missing secret", testBFFSecret, "", forwarded, "secret_mismatch"},
		{"invalid address", testBFFSecret, testBFFSecret, "not-an-ip", "invalid_address"},
		{"unspecified address", testBFFSecret, testBFFSecret, "0.0.0.0", "invalid_address"},
		{"unspecified IPv6 address", testBFFSecret, testBFFSecret, "::", "invalid_address"},
		{"multicast address", testBFFSecret, testBFFSecret, "224.0.0.1", "invalid_address"},
		{"multicast IPv6 address", testBFFSecret, testBFFSecret, "ff02::1", "invalid_address"},
		{"empty configured secret", "", sentSecret, forwarded, ""},
		{"no client IP header", testBFFSecret, sentSecret, "", ""},
		{"accepted address", testBFFSecret, testBFFSecret, forwarded, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			e := newBFFEcho(tt.configured, slog.New(slog.NewTextHandler(&buf, nil)))
			req := bffRequest(tt.sentSecret, tt.sentClientIP)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			log := buf.String()
			if tt.wantReason == "" {
				if log != "" {
					t.Fatalf("want no log, got %q", log)
				}
				return
			}
			if n := strings.Count(log, "level=WARN"); n != 1 {
				t.Fatalf("want one WARN line, got %d: %q", n, log)
			}
			for _, want := range []string{"bff client ip rejected", "reason=" + tt.wantReason} {
				if !strings.Contains(log, want) {
					t.Errorf("log %q does not contain %q", log, want)
				}
			}
			secrets := []string{tt.configured, tt.sentSecret, tt.sentClientIP}
			for _, secret := range secrets {
				if secret != "" && strings.Contains(log, secret) {
					t.Errorf("log leaks %q: %q", secret, log)
				}
			}
		})
	}
}

func TestBFFClientIP_warningCarriesRequestID(t *testing.T) {
	var buf bytes.Buffer
	e := newBFFEcho(testBFFSecret, slog.New(slog.NewTextHandler(&buf, nil)), middleware.RequestID())
	rec := httptest.NewRecorder()
	req := bffRequest("wrong", "198.51.100.7")
	req.Header.Set(echo.HeaderXRequestID, "req-42")
	e.ServeHTTP(rec, req)
	if !strings.Contains(buf.String(), "request_id=req-42") {
		t.Errorf("want request_id in the warning, got %q", buf.String())
	}
}

func TestClientIP_withoutMiddlewareReturnsRealIP(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()
	e.GET("/ip", func(c echo.Context) error {
		return c.String(http.StatusOK, ClientIP(c))
	})
	req := bffRequest(testBFFSecret, "198.51.100.7")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != testPeerIP {
		t.Errorf("ClientIP = %q, want %q", got, testPeerIP)
	}
}

func TestRequestLog_clientIP(t *testing.T) {
	tests := []struct {
		name       string
		sentSecret string
		wantClient bool
	}{
		{"accepted BFF address is logged", testBFFSecret, true},
		{"rejected BFF address is not logged", "wrong", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			e := newBFFEcho(testBFFSecret, nil, RequestLog(logger))
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, bffRequest(tt.sentSecret, "198.51.100.7"))

			log := buf.String()
			if !strings.Contains(log, "remote_ip="+testPeerIP) {
				t.Errorf("want remote_ip=%s, got %q", testPeerIP, log)
			}
			if got := strings.Contains(log, "client_ip=198.51.100.7"); got != tt.wantClient {
				t.Errorf("client_ip logged = %v, want %v: %q", got, tt.wantClient, log)
			}
			if !tt.wantClient && strings.Contains(log, "client_ip=") {
				t.Errorf("unexpected client_ip key: %q", log)
			}
			if tt.wantClient && strings.Index(log, "client_ip=") < strings.Index(log, "remote_ip=") {
				t.Errorf("client_ip must follow remote_ip: %q", log)
			}
		})
	}
}
