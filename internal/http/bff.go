package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/netip"
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	// HeaderBFFClientIP carries the browser address that the dashboard's server forwards.
	HeaderBFFClientIP = "X-Mentorix-Client-IP"
	// HeaderBFFSecret proves the request comes from the dashboard's server.
	HeaderBFFSecret = "X-Mentorix-BFF-Secret"

	bffClientIPKey = "mentorix.bff_client_ip"
)

// BFFClientIP accepts the browser address in HeaderBFFClientIP when HeaderBFFSecret
// equals secret. An empty secret disables it. A rejected address is logged without
// the secrets or the address and the request goes on with the address Echo found.
func BFFClientIP(secret string, logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		if secret == "" {
			return next
		}
		// Comparing fixed-size digests keeps the comparison time independent of
		// the length of the received secret.
		want := sha256.Sum256([]byte(secret))
		return func(c echo.Context) error {
			req := c.Request()
			forwarded := req.Header.Get(HeaderBFFClientIP)
			if forwarded == "" {
				return next(c)
			}
			got := sha256.Sum256([]byte(req.Header.Get(HeaderBFFSecret)))
			if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				warnBFFRejected(c, logger, "secret_mismatch")
				return next(c)
			}
			addr, err := netip.ParseAddr(strings.TrimSpace(forwarded))
			if err != nil {
				warnBFFRejected(c, logger, "invalid_address")
				return next(c)
			}
			addr = addr.WithZone("").Unmap()
			// Loopback and private addresses stay valid: local development runs
			// the BFF on 127.0.0.1.
			if addr.IsUnspecified() || addr.IsMulticast() {
				warnBFFRejected(c, logger, "invalid_address")
				return next(c)
			}
			c.Set(bffClientIPKey, addr.String())
			return next(c)
		}
	}
}

func warnBFFRejected(c echo.Context, logger *slog.Logger, reason string) {
	if logger == nil {
		return
	}
	logger.Warn("bff client ip rejected",
		"reason", reason,
		"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
	)
}

// ClientIP is the address the auth rate limits count by: the browser address that
// BFFClientIP accepted, else c.RealIP(). Without the middleware it is c.RealIP().
func ClientIP(c echo.Context) string {
	if ip, ok := bffClientIP(c); ok {
		return ip
	}
	return c.RealIP()
}

func bffClientIP(c echo.Context) (string, bool) {
	ip, ok := c.Get(bffClientIPKey).(string)
	return ip, ok && ip != ""
}
