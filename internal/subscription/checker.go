package subscription

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

// Checker resolves quota checks by trainer user id for other feature services.
type Checker struct {
	q *sqlc.Queries
}

func NewChecker(pool *pgxpool.Pool) *Checker {
	return &Checker{q: sqlc.New(pool)}
}

// CheckQuota verifies the quota for the user's trainer profile.
// Users without a trainer profile pass: ownership checks gate their access.
func (c *Checker) CheckQuota(ctx context.Context, trainerUserID uuid.UUID, resource Resource, op Op) error {
	trainerID, err := c.q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("trainer lookup: %w", err)
	}
	return CheckQuota(ctx, c.q, pgconv.FromPGUUID(trainerID), resource, op)
}
