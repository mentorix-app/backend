package subscription

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

// Op distinguishes quota-increasing operations from mutations of existing resources.
type Op int

const (
	// OpCreate is blocked when usage has reached the limit.
	OpCreate Op = iota
	// OpMutate is blocked only when usage is above the limit (read-only after downgrade).
	OpMutate
)

// EffectivePlan resolves the highest active plan of a trainer ("max active tier" rule).
// Works with both pool-backed and tx-backed Queries.
func EffectivePlan(ctx context.Context, q *sqlc.Queries, trainerID uuid.UUID) (PlanInfo, error) {
	rows, err := q.ListActiveTrainerPlanEntitlements(ctx, pgconv.ToPGUUID(trainerID))
	if err != nil {
		return PlanInfo{}, fmt.Errorf("list entitlements: %w", err)
	}
	info := PlanInfo{Plan: PlanFree}
	best := -1
	for _, row := range rows {
		p := Plan(row.PlanCode)
		if !p.Valid() || p.rank() <= best {
			continue
		}
		best = p.rank()
		src := Source(row.Source)
		info = PlanInfo{Plan: p, Source: &src}
		if row.ValidUntil.Valid {
			t := row.ValidUntil.Time.UTC()
			info.ValidUntil = &t
		} else {
			info.ValidUntil = nil
		}
	}
	return info, nil
}

// TrainerUsage counts a trainer's quota-relevant resources in one query.
func TrainerUsage(ctx context.Context, q *sqlc.Queries, trainerID uuid.UUID) (Usage, error) {
	row, err := q.GetTrainerUsage(ctx, pgconv.ToPGUUID(trainerID))
	if err != nil {
		return Usage{}, fmt.Errorf("trainer usage: %w", err)
	}
	return Usage{
		Exercises:      int(row.Exercises),
		ActivePrograms: int(row.ActivePrograms),
		ActiveClients:  int(row.ActiveClients),
	}, nil
}

// LockTrainer takes a row lock on the trainer to serialize quota-increasing operations.
// Must be called inside a transaction.
func LockTrainer(ctx context.Context, q *sqlc.Queries, trainerID uuid.UUID) error {
	if _, err := q.LockTrainerRow(ctx, pgconv.ToPGUUID(trainerID)); err != nil {
		return fmt.Errorf("lock trainer: %w", err)
	}
	return nil
}

// CheckQuota verifies that op on resource is allowed under the trainer's effective plan.
// Returns *QuotaError when blocked.
func CheckQuota(ctx context.Context, q *sqlc.Queries, trainerID uuid.UUID, resource Resource, op Op) error {
	info, err := EffectivePlan(ctx, q, trainerID)
	if err != nil {
		return err
	}
	limit := limitFor(info.Plan, resource)
	if limit == nil {
		return nil
	}
	usage, err := TrainerUsage(ctx, q, trainerID)
	if err != nil {
		return err
	}
	current := usageFor(usage, resource)
	blocked := false
	switch op {
	case OpCreate:
		blocked = current >= *limit
	case OpMutate:
		blocked = current > *limit
	}
	if blocked {
		return &QuotaError{Resource: resource, Plan: info.Plan, Limit: *limit, Usage: current}
	}
	return nil
}

func limitFor(p Plan, resource Resource) *int {
	limits := planLimits(p)
	switch resource {
	case ResourceExercises:
		return limits.Exercises
	case ResourcePrograms:
		return limits.ActivePrograms
	case ResourceClients:
		return limits.ActiveClients
	default:
		return nil
	}
}

func usageFor(u Usage, resource Resource) int {
	switch resource {
	case ResourceExercises:
		return u.Exercises
	case ResourcePrograms:
		return u.ActivePrograms
	case ResourceClients:
		return u.ActiveClients
	default:
		return 0
	}
}
