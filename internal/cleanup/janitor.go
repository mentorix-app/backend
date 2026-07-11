package cleanup

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/db/sqlc"
)

type Result struct {
	TrainerInvites  int64
	RefreshSessions int64
}

func Run(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	q := sqlc.New(pool)

	invites, err := q.PurgeStaleTrainerInvites(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("purge trainer invites: %w", err)
	}

	sessions, err := q.PurgeStaleRefreshSessions(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("purge refresh sessions: %w", err)
	}

	return Result{
		TrainerInvites:  invites,
		RefreshSessions: sessions,
	}, nil
}
