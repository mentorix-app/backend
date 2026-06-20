package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

const pingTimeout = 2 * time.Second

const (
	StatusOK                    = "ok"
	CheckError                  = "error"
	CheckSkipped                = "skipped"
	ReadyStatusReady            = "ready"
	ReadyStatusNotReady         = "not_ready"
	ReadyStatusNoDepsConfigured = "no_dependencies_configured"
)

type ReadyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func RegisterReady(e *echo.Echo, pool *pgxpool.Pool, rdb *redis.Client) {
	e.GET("/health/ready", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), pingTimeout)
		defer cancel()

		checks := make(map[string]string, 2)
		anyRequired := false
		allOK := true

		if pool != nil {
			anyRequired = true
			if err := pool.Ping(ctx); err != nil {
				checks["database"] = CheckError
				allOK = false
			} else {
				checks["database"] = StatusOK
			}
		} else {
			checks["database"] = CheckSkipped
		}

		if rdb != nil {
			anyRequired = true
			if err := rdb.Ping(ctx).Err(); err != nil {
				checks["redis"] = CheckError
				allOK = false
			} else {
				checks["redis"] = StatusOK
			}
		} else {
			checks["redis"] = CheckSkipped
		}

		status := ReadyStatusReady
		if anyRequired && !allOK {
			status = ReadyStatusNotReady
		}
		if !anyRequired {
			status = ReadyStatusNoDepsConfigured
		}

		code := http.StatusOK
		if anyRequired && !allOK {
			code = http.StatusServiceUnavailable
		}

		return c.JSON(code, ReadyResponse{Status: status, Checks: checks})
	})
}

func NewRedisClient(url string) (*redis.Client, error) {
	if url == "" {
		return nil, nil
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis url: %w", err)
	}
	return redis.NewClient(opts), nil
}

func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, nil
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return pool, nil
}
