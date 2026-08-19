package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"mentorix-backend/internal/cleanup"
	"mentorix-backend/internal/config"
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
	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	result, err := cleanup.Run(ctx, pool)
	if err != nil {
		logger.Error("janitor failed", "error", err)
		os.Exit(1)
	}

	logger.Info("janitor completed",
		"trainer_invites_deleted", result.TrainerInvites,
		"refresh_sessions_deleted", result.RefreshSessions,
		"program_block_clients_deleted", result.ProgramBlockClients,
	)
}
