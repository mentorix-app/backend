// Package app wires the HTTP API: it is the one place that builds the feature
// services and mounts their handlers, shared by cmd/api and the OpenAPI
// contract check so a handler cannot ship without being checked.
package app

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"mentorix-backend/internal/admin"
	"mentorix-backend/internal/analytics"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/config"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/health"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
	"mentorix-backend/internal/telegram"
	"mentorix-backend/internal/telegramnotify"
	"mentorix-backend/internal/trainerclient"
	"mentorix-backend/internal/workoutcomment"
)

// Deps are the collaborators Mount needs. Pool and Redis may be nil.
type Deps struct {
	Config config.Config
	Pool   *pgxpool.Pool
	Redis  *redis.Client
	Logger *slog.Logger
	// RouteDiscovery mounts the database-backed routes even when Pool is nil.
	// The contract check sets it; the server leaves it off so a missing
	// DATABASE_URL disables those routes.
	RouteDiscovery bool
}

// Result carries what the caller needs after mounting.
type Result struct {
	// Notifier is non-nil when a bot token is configured; the caller closes it
	// on shutdown.
	Notifier *telegramnotify.Notifier
	// TrainerClient is non-nil whenever the database-backed routes are mounted.
	TrainerClient *trainerclient.Service
}

// Mount registers every HTTP route that is part of the OpenAPI contract. The
// Telegram webhook is deliberately not here: it is outside OpenAPI and stays
// mounted by cmd/api.
func Mount(e *echo.Echo, d Deps) Result {
	cfg := d.Config
	health.RegisterLiveness(e, cfg.GitCommit)
	health.RegisterReady(e, d.Pool, d.Redis)

	if d.Pool == nil && !d.RouteDiscovery {
		if d.Logger != nil {
			d.Logger.Warn("DATABASE_URL not set; auth, exercise, and program routes are disabled")
		}
		return Result{}
	}
	pool := d.Pool

	var res Result
	limiter := auth.NewRateLimiter(
		d.Redis,
		cfg.AuthLoginRateMax,
		cfg.AuthLoginRateWindow,
		cfg.AuthRegisterRateMax,
		cfg.AuthRegisterRateWin,
	).WithLogger(d.Logger)
	subsSvc := subscription.NewService(pool)
	subscription.NewHandlers(auth.JWTMiddleware(cfg.JWTSecret)).Mount(e)

	var authOpts []auth.ServiceOption
	if len(cfg.GoogleClientIDs) > 0 {
		authOpts = append(authOpts, auth.WithGoogleVerifier(auth.NewGoogleIDTokenVerifier(cfg.GoogleClientIDs)))
	}
	if len(cfg.AppleClientIDs) > 0 {
		authOpts = append(authOpts, auth.WithAppleVerifier(auth.NewAppleIDTokenVerifier(cfg.AppleClientIDs)))
	}
	svc := auth.NewService(pool, cfg.JWTSecret, cfg.AccessTokenTTL(), cfg.RefreshTokenTTL(), authOpts...)
	auth.NewHandlers(svc, cfg.JWTSecret, cfg.RefreshCookie, cfg.RefreshTokenTTL(), limiter,
		auth.WithSubscriptions(subsSvc),
	).Mount(e)

	adminSvc := admin.NewService(pool)
	admin.NewHandlers(adminSvc, pool, cfg.JWTSecret).Mount(e)

	exSvc := exercise.NewService(pool)
	exercise.NewHandlers(exSvc, pool, cfg.JWTSecret).Mount(e)

	var programNotifier program.ProgramNotifier
	var trainerNotifier trainerclient.ProgramNotifier
	var commentNotifier workoutcomment.CommentNotifier
	if cfg.BotToken != "" {
		n := telegramnotify.NewNotifier(pool, telegramnotify.NewSender(cfg.BotToken), d.Logger)
		res.Notifier = n
		programNotifier = n
		trainerNotifier = n
		commentNotifier = n
	}
	progSvc := program.NewService(pool, program.WithProgramNotifier(programNotifier))

	program.NewHandlers(progSvc, pool, cfg.JWTSecret).Mount(e)

	var photoClient trainerclient.ProfilePhotoFetcher
	if cfg.BotToken != "" {
		photoClient = telegram.NewProfilePhotoClient(cfg.BotToken)
	}
	res.TrainerClient = trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: cfg.TelegramBotUsername,
		InviteTTL:           cfg.TrainerInviteTTL(),
	}, trainerclient.NewActiveTrainerStore(d.Redis), trainerNotifier,
		trainerclient.WithAvatarSupport(cfg.JWTSecret, cfg.BotToken, photoClient),
	)
	trainerclient.NewHandlers(res.TrainerClient, pool, cfg.JWTSecret).Mount(e)

	analyticsSvc := analytics.NewService(pool, cfg.JWTSecret)
	analytics.NewHandlers(analyticsSvc, pool, cfg.JWTSecret).Mount(e)

	commentSvc := workoutcomment.NewService(pool, workoutcomment.WithCommentNotifier(commentNotifier))
	workoutcomment.NewHandlers(commentSvc, pool, cfg.JWTSecret).Mount(e)

	return res
}
