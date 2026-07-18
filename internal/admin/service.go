package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/subscription"
)

type planService interface {
	GrantAdminPlan(ctx context.Context, trainerUserID uuid.UUID, plan subscription.Plan) (*subscription.Subscription, error)
	RevokeAdminPlan(ctx context.Context, trainerUserID uuid.UUID) (*subscription.Subscription, error)
}

type Service struct {
	plans planService
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{plans: subscription.NewService(pool)}
}

// GrantPlan issues a perpetual admin grant (advance/elite) to a trainer;
// "free" removes the grant.
func (s *Service) GrantPlan(ctx context.Context, trainerUserID uuid.UUID, plan subscription.Plan) (*subscription.Subscription, error) {
	return s.plans.GrantAdminPlan(ctx, trainerUserID, plan)
}

// RevokePlan removes the active admin grant of a trainer.
func (s *Service) RevokePlan(ctx context.Context, trainerUserID uuid.UUID) (*subscription.Subscription, error) {
	return s.plans.RevokeAdminPlan(ctx, trainerUserID)
}
