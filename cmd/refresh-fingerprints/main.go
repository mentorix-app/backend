package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/config"
	"mentorix-backend/internal/program"
)

// runTimeout bounds the whole run so a stuck database cannot hold a deploy.
const runTimeout = 2 * time.Minute

func main() {
	config.LoadDotenv()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Not config.Load: it requires app secrets that a deploy step does not have.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Error("database connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	updated, err := program.NewStore(pool).RefreshVersionFingerprints(ctx)
	if err != nil {
		logger.Error("version fingerprint refresh failed", "error", err)
		os.Exit(1)
	}

	logger.Info("version fingerprints refreshed", "updated", updated)
}
