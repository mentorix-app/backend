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
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/config"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/health"
	apphttp "mentorix-backend/internal/http"
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

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	health.RegisterReady(e, pool, rdb)

	if pool != nil {
		limiter := auth.NewRateLimiter(
			rdb,
			cfg.AuthLoginRateMax,
			cfg.AuthLoginRateWindow,
			cfg.AuthRegisterRateMax,
			cfg.AuthRegisterRateWin,
		)
		svc := auth.NewService(pool, cfg.JWTSecret, cfg.AccessTokenTTL(), cfg.RefreshTokenTTL())
		auth.NewHandlers(svc, cfg.JWTSecret, cfg.RefreshCookie, cfg.RefreshTokenTTL(), limiter).Mount(e)

		adminSvc := admin.NewService(pool)
		admin.NewHandlers(adminSvc, pool, cfg.JWTSecret).Mount(e)

		exSvc := exercise.NewService(pool)
		exercise.NewHandlers(exSvc, pool, cfg.JWTSecret).Mount(e)
	} else {
		logger.Warn("DATABASE_URL not set; auth and exercise routes are disabled")
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
