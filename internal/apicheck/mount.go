package apicheck

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/app"
	"mentorix-backend/internal/config"
)

const contractJWTSecret = "contract-check-jwt-secret-min-32-chars"

// MountRoutes registers the full HTTP API through app.Mount, the same function
// cmd/api uses. pool may be nil; handlers are only registered for route discovery.
func MountRoutes(e *echo.Echo, pool *pgxpool.Pool) {
	app.Mount(e, app.Deps{
		Config: config.Config{
			JWTSecret:        contractJWTSecret,
			AccessTTLMinutes: 15,
			RefreshTTLDays:   30,
			RefreshCookie: config.RefreshCookieSettings{
				Name:     "mentorix_refresh",
				Path:     "/",
				SameSite: http.SameSiteLaxMode,
			},
			TelegramBotUsername:  "mentorix_bot",
			TrainerInviteTTLDays: 7,
		},
		Pool:           pool,
		RouteDiscovery: true,
	})
}
