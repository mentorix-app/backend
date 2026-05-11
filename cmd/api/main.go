package main

import (
	"context"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/config"
	"mentorix-backend/internal/health"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := health.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	if pool != nil {
		defer pool.Close()
	}

	rdb, err := health.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatal(err)
	}
	if rdb != nil {
		defer rdb.Close()
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

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
		svc := auth.NewService(pool, cfg.JWTSecret)
		auth.NewHandlers(svc, cfg.JWTSecret).Mount(e)
	}

	addr := ":" + cfg.Port
	e.Logger.Fatal(e.Start(addr))
}
