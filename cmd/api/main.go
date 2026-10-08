package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"mentorix-backend/internal/analytics"
	"mentorix-backend/internal/app"
	"mentorix-backend/internal/config"
	"mentorix-backend/internal/health"
	apphttp "mentorix-backend/internal/http"
	"mentorix-backend/internal/telegrambot"
	"mentorix-backend/internal/trainerclient"
	"mentorix-backend/internal/workoutcompletion"
)

const (
	readTimeout  = 15 * time.Second
	writeTimeout = 15 * time.Second
	idleTimeout  = 60 * time.Second
	bodyLimit    = "1M"
	shutdownTTL  = 10 * time.Second

	notifierCloseTTL = 10 * time.Second

	webhookRetryPause = 30 * time.Second
)

func main() {
	config.LoadDotenv()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	lifetimeCtx, cancelLifetime := context.WithCancel(ctx)
	defer cancelLifetime()
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

	mounted := app.Mount(e, app.Deps{
		Config: cfg,
		Pool:   pool,
		Redis:  rdb,
		Logger: logger,
	})
	notifier := mounted.Notifier

	if pool != nil {
		// The webhook is outside OpenAPI, so it is mounted here and not by app.Mount.
		if cfg.BotToken != "" && cfg.BotWebhookURL != "" {
			workoutSvc := workoutcompletion.NewService(pool)
			workoutPending := workoutcompletion.NewPendingStore(rdb)
			botOpts := []telegrambot.BotOption{
				telegrambot.WithWorkoutCompletions(workoutSvc, workoutPending),
				telegrambot.WithAvatarCheckStore(trainerclient.NewAvatarCheckStore(rdb)),
			}
			if cfg.ClientAnalyticsPageURL != "" {
				botOpts = append(botOpts, telegrambot.WithClientAnalyticsLink(
					analytics.NewClientLinkBuilder(cfg.ClientAnalyticsPageURL, cfg.JWTSecret),
				))
			} else {
				logger.Info("client analytics page not configured; stats button disabled")
			}
			tgBot := telegrambot.NewFromToken(cfg.BotToken, mounted.TrainerClient, botOpts...)
			e.POST("/telegram/webhook", telegrambot.WebhookHandler(cfg.BotWebhookSecret, tgBot))
			go telegrambot.RegisterWebhookWithRetry(lifetimeCtx, logger, webhookRetryPause, cfg.BotWebhookURL, func(ctx context.Context) error {
				return tgBot.RegisterWebhook(ctx, cfg.BotToken, telegrambot.WebhookConfig{
					URL:         cfg.BotWebhookURL,
					SecretToken: cfg.BotWebhookSecret,
				})
			})
		} else if cfg.BotToken != "" {
			logger.Warn("telegram bot webhook not configured; push enabled, incoming updates disabled")
		}
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
	cancelLifetime()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTTL)
	defer cancel()
	shutdownErr := e.Shutdown(shutdownCtx)

	// Requests have stopped, so no new notifications arrive; send what is
	// queued before the deferred pool close.
	closeCtx, cancelClose := context.WithTimeout(context.Background(), notifierCloseTTL)
	defer cancelClose()
	if err := notifier.Close(closeCtx); err != nil {
		logger.Error("notifier close failed", "error", err)
	}

	if shutdownErr != nil {
		logger.Error("shutdown failed", "error", shutdownErr)
		os.Exit(1)
	}
}
