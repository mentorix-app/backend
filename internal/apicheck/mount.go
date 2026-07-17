package apicheck

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/admin"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/config"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/health"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
	"mentorix-backend/internal/trainerclient"
)

const contractJWTSecret = "contract-check-jwt-secret-min-32-chars"

// MountRoutes registers the full HTTP API the same way as cmd/api when DATABASE_URL is set.
// pool may be nil; handlers are only registered for route discovery.
func MountRoutes(e *echo.Echo, pool *pgxpool.Pool) {
	health.RegisterLiveness(e)
	health.RegisterReady(e, pool, nil)

	cookie := config.RefreshCookieSettings{
		Name:     "mentorix_refresh",
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	}
	limiter := auth.NewRateLimiter(nil, 0, 0, 0, 0)
	accessTTL := 15 * time.Minute
	refreshTTL := 30 * 24 * time.Hour

	subscription.NewHandlers(auth.JWTMiddleware(contractJWTSecret)).Mount(e)

	authSvc := auth.NewService(pool, contractJWTSecret, accessTTL, refreshTTL)
	auth.NewHandlers(authSvc, contractJWTSecret, cookie, refreshTTL, limiter).Mount(e)

	adminSvc := admin.NewService(pool)
	admin.NewHandlers(adminSvc, pool, contractJWTSecret).Mount(e)

	exSvc := exercise.NewService(pool)
	exercise.NewHandlers(exSvc, pool, contractJWTSecret).Mount(e)

	progSvc := program.NewService(pool)
	program.NewHandlers(progSvc, pool, contractJWTSecret).Mount(e)

	trainerClientSvc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)
	trainerclient.NewHandlers(trainerClientSvc, pool, contractJWTSecret).Mount(e)
}
