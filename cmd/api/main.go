package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"mentorix-backend/internal/admin"
	"mentorix-backend/internal/analytics"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/config"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/health"
	apphttp "mentorix-backend/internal/http"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
	"mentorix-backend/internal/telegram"
	"mentorix-backend/internal/telegrambot"
	"mentorix-backend/internal/telegramnotify"
	"mentorix-backend/internal/trainerclient"
	"mentorix-backend/internal/workoutcomment"
	"mentorix-backend/internal/workoutcompletion"
)

const (
	readTimeout  = 15 * time.Second
	writeTimeout = 15 * time.Second
	idleTimeout  = 60 * time.Second
	bodyLimit    = "1M"
	shutdownTTL  = 10 * time.Second
)

func main() {
	_ = godotenv.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := health.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database pool failed", "error", err)
		os.Exit(1)
	}
	if pool != nil {
		defer func() {
			pool.Close()
		}()
	}

	rdb, err := health.NewRedisClient(cfg.RedisURL)
	if err != nil {
		logger.Error("redis client failed", "error", err)
		os.Exit(1)
	}
	if rdb != nil {
		defer func() {
			if err := rdb.Close(); err != nil {
				logger.Error("redis close failed", "error", err)
			}
		}()
	}

	e := echo.New()
	apphttp.ConfigureIPExtractor(e, cfg.TrustedProxyCIDRs)
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.RequestID())
	e.Use(apphttp.RequestLog(logger))
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit(bodyLimit))

	if len(cfg.CORSAllowedOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:     cfg.CORSAllowedOrigins,
			AllowMethods:     []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
			AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
			AllowCredentials: true,
			MaxAge:           86400,
		}))
	}

	health.RegisterLiveness(e)
	health.RegisterReady(e, pool, rdb)

	if pool != nil {
		limiter := auth.NewRateLimiter(
			rdb,
			cfg.AuthLoginRateMax,
			cfg.AuthLoginRateWindow,
			cfg.AuthRegisterRateMax,
			cfg.AuthRegisterRateWin,
		)
		subsSvc := subscription.NewService(pool)
		subscription.NewHandlers(auth.JWTMiddleware(cfg.JWTSecret)).Mount(e)

		svc := auth.NewService(pool, cfg.JWTSecret, cfg.AccessTokenTTL(), cfg.RefreshTokenTTL())
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
			sender, err := telegramnotify.NewSender(cfg.BotToken)
			if err != nil {
				logger.Warn("telegram push disabled", "error", err)
			} else {
				n := telegramnotify.NewNotifier(pool, sender, logger)
				programNotifier = n
				trainerNotifier = n
				commentNotifier = n
			}
		}
		progSvc := program.NewService(pool, program.WithProgramNotifier(programNotifier))

		program.NewHandlers(progSvc, pool, cfg.JWTSecret).Mount(e)

		var photoClient trainerclient.ProfilePhotoFetcher
		if cfg.BotToken != "" {
			pc, err := telegram.NewProfilePhotoClient(cfg.BotToken)
			if err != nil {
				logger.Warn("telegram profile photos disabled", "error", err)
			} else {
				photoClient = pc
			}
		}
		trainerClientSvc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
			TelegramBotUsername: cfg.TelegramBotUsername,
			InviteTTL:           cfg.TrainerInviteTTL(),
		}, trainerclient.NewActiveTrainerStore(rdb), trainerNotifier,
			trainerclient.WithAvatarSupport(cfg.JWTSecret, cfg.BotToken, photoClient),
		)
		trainerclient.NewHandlers(trainerClientSvc, pool, cfg.JWTSecret).Mount(e)

		analyticsSvc := analytics.NewService(pool, cfg.JWTSecret)
		analytics.NewHandlers(analyticsSvc, pool, cfg.JWTSecret).Mount(e)

		commentSvc := workoutcomment.NewService(pool, workoutcomment.WithCommentNotifier(commentNotifier))
		workoutcomment.NewHandlers(commentSvc, pool, cfg.JWTSecret).Mount(e)

		workoutSvc := workoutcompletion.NewService(pool)
		workoutPending := workoutcompletion.NewPendingStore(rdb)

		if cfg.BotToken != "" && cfg.BotWebhookURL != "" {
			tgBot, err := telegrambot.NewFromToken(cfg.BotToken, trainerClientSvc,
				telegrambot.WithWorkoutCompletions(workoutSvc, workoutPending),
			)
			if err != nil {
				logger.Error("telegram webhook bot init failed", "error", err)
				os.Exit(1)
			}
			e.POST("/telegram/webhook", telegrambot.WebhookHandler(cfg.BotWebhookSecret, tgBot))
			if err := tgBot.RegisterWebhook(cfg.BotToken, telegrambot.WebhookConfig{
				URL:         cfg.BotWebhookURL,
				SecretToken: cfg.BotWebhookSecret,
			}); err != nil {
				logger.Error("telegram webhook registration failed", "error", err)
				os.Exit(1)
			}
			logger.Info("telegram webhook registered", "url", cfg.BotWebhookURL)
		} else if cfg.BotToken != "" {
			logger.Warn("telegram bot webhook not configured; push enabled, incoming updates disabled")
		}
	} else {
		logger.Warn("DATABASE_URL not set; auth, exercise, and program routes are disabled")
	}

	addr := ":" + cfg.Port
	server := &http.Server{
		Addr:         addr,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	go func() {
		if err := e.StartServer(server); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("server started", "addr", addr, "app_env", cfg.AppEnv)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTTL)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
}
