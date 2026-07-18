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

type Service struct {
	q *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: sqlc.New(pool)}
}

// ForUser builds the full subscription state for /auth/me.
// Returns nil for users without a trainer profile (admins, telegram-only clients).
func (s *Service) ForUser(ctx context.Context, userID uuid.UUID) (*Subscription, error) {
	trainerID, err := s.q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("trainer lookup: %w", err)
	}
	return s.forTrainer(ctx, pgconv.FromPGUUID(trainerID))
}

func (s *Service) forTrainer(ctx context.Context, trainerID uuid.UUID) (*Subscription, error) {
	info, err := EffectivePlan(ctx, s.q, trainerID)
	if err != nil {
		return nil, err
	}
	usage, err := TrainerUsage(ctx, s.q, trainerID)
	if err != nil {
		return nil, err
	}
	limits := planLimits(info.Plan)
	return &Subscription{
		Plan:        info.Plan,
		Source:      info.Source,
		ValidUntil:  info.ValidUntil,
		Limits:      limits,
		Usage:       usage,
		Permissions: permissions(limits, usage),
	}, nil
}

// GrantAdminPlan sets or replaces the perpetual admin-issued grant for a trainer.
func (s *Service) GrantAdminPlan(ctx context.Context, trainerUserID uuid.UUID, plan Plan) (*Subscription, error) {
	if !plan.Valid() {
		return nil, ErrInvalidPlan
	}
	trainerID, err := s.trainerID(ctx, trainerUserID)
	if err != nil {
		return nil, err
	}
	if plan == PlanFree {
		// Free is the default: an explicit free grant is just a revoke.
		if _, err := s.q.RevokeAdminPlanGrant(ctx, pgconv.ToPGUUID(trainerID)); err != nil {
			return nil, fmt.Errorf("revoke admin grant: %w", err)
		}
	} else if _, err := s.q.UpsertAdminPlanGrant(ctx, sqlc.UpsertAdminPlanGrantParams{
		TrainerID: pgconv.ToPGUUID(trainerID),
		PlanCode:  string(plan),
	}); err != nil {
		return nil, fmt.Errorf("upsert admin grant: %w", err)
	}
	return s.forTrainer(ctx, trainerID)
}

// RevokeAdminPlan removes the active admin-issued grant; the trainer falls back
// to the highest remaining entitlement or Free.
func (s *Service) RevokeAdminPlan(ctx context.Context, trainerUserID uuid.UUID) (*Subscription, error) {
	trainerID, err := s.trainerID(ctx, trainerUserID)
	if err != nil {
		return nil, err
	}
	if _, err := s.q.RevokeAdminPlanGrant(ctx, pgconv.ToPGUUID(trainerID)); err != nil {
		return nil, fmt.Errorf("revoke admin grant: %w", err)
	}
	return s.forTrainer(ctx, trainerID)
}

func (s *Service) trainerID(ctx context.Context, trainerUserID uuid.UUID) (uuid.UUID, error) {
	trainerID, err := s.q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrTrainerNotFound
		}
		return uuid.Nil, fmt.Errorf("trainer lookup: %w", err)
	}
	return pgconv.FromPGUUID(trainerID), nil
}

func permissions(limits Limits, usage Usage) Permissions {
	return Permissions{
		CanCreateExercise: canCreate(limits.Exercises, usage.Exercises),
		CanEditExercises:  canMutate(limits.Exercises, usage.Exercises),
		CanCreateProgram:  canCreate(limits.ActivePrograms, usage.ActivePrograms),
		CanEditPrograms:   canMutate(limits.ActivePrograms, usage.ActivePrograms),
		CanCreateInvite:   canCreate(limits.ActiveClients, usage.ActiveClients),
		CanManageClients:  canMutate(limits.ActiveClients, usage.ActiveClients),
	}
}

func canCreate(limit *int, usage int) bool {
	return limit == nil || usage < *limit
}

func canMutate(limit *int, usage int) bool {
	return limit == nil || usage <= *limit
}
