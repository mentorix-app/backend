package http

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func RequestLog(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			req := c.Request()
			status := responseStatus(c, err)
			attrs := []any{
				"method", req.Method,
				"path", req.URL.Path,
				"status", status,
				"latency_ms", time.Since(start).Milliseconds(),
				"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
				"remote_ip", c.RealIP(),
			}
			switch {
			case status >= http.StatusInternalServerError:
				if err != nil {
					attrs = append(attrs, "error", err.Error())
				}
				logger.Error("request", attrs...)
			case status >= http.StatusBadRequest:
				if err != nil {
					attrs = append(attrs, "error", err.Error())
				}
				logger.Warn("request", attrs...)
			default:
				logger.Info("request", attrs...)
			}
			return err
		}
	}
}

func responseStatus(c echo.Context, err error) int {
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) && he.Code > 0 {
			return he.Code
		}
		if status := c.Response().Status; status >= http.StatusBadRequest {
			return status
		}
		return http.StatusInternalServerError
	}
	if status := c.Response().Status; status > 0 {
		return status
	}
	return http.StatusOK
}

func ConfigureIPExtractor(e *echo.Echo, trustedCIDRs []string) {
	if len(trustedCIDRs) == 0 {
		e.IPExtractor = echo.ExtractIPDirect()
		return
	}

	opts := make([]echo.TrustOption, 0, len(trustedCIDRs))
	for _, raw := range trustedCIDRs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.EqualFold(raw, "private") {
			opts = append(opts, echo.TrustPrivateNet(true))
			continue
		}
		if strings.EqualFold(raw, "loopback") {
			opts = append(opts, echo.TrustLoopback(true))
			continue
		}
		if _, network, err := net.ParseCIDR(raw); err == nil {
			opts = append(opts, echo.TrustIPRange(network))
			continue
		}
		if ip := net.ParseIP(raw); ip != nil {
			mask := net.CIDRMask(len(ip)*8, len(ip)*8)
			opts = append(opts, echo.TrustIPRange(&net.IPNet{IP: ip, Mask: mask}))
		}
	}

	if len(opts) == 0 {
		e.IPExtractor = echo.ExtractIPDirect()
		return
	}
	e.IPExtractor = echo.ExtractIPFromXFFHeader(opts...)
}
