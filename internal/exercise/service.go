package exercise

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/subscription"
)

// ErrForbidden is returned when the acting user may not touch the exercise.
var ErrForbidden = errors.New("forbidden")

type exerciseStore interface {
	List(ctx context.Context, viewer Viewer, params ListParams) (ListResult, error)
	GetByID(ctx context.Context, id uuid.UUID) (Exercise, error)
	Create(ctx context.Context, userID uuid.UUID, ownerTrainerID *uuid.UUID, in UpsertInput) (Exercise, error)
	Update(ctx context.Context, id, userID uuid.UUID, ownerTrainerID *uuid.UUID, in UpsertInput) (Exercise, error)
	DeleteMany(ctx context.Context, userID uuid.UUID, ownerTrainerID *uuid.UUID, ids []uuid.UUID) (int64, error)
	TrainerIDForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}

// QuotaChecker mirrors program.QuotaChecker; nil disables checks (tests).
type QuotaChecker interface {
	CheckQuota(ctx context.Context, trainerUserID uuid.UUID, resource subscription.Resource, op subscription.Op) error
}

type Service struct {
	store exerciseStore
	roles auth.RoleQuerier
	quota QuotaChecker
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		store: NewStore(pool),
		roles: sqlc.New(pool),
		quota: subscription.NewChecker(pool),
	}
}

// NewServiceWithStore wires a Service with test or custom implementations.
func NewServiceWithStore(store exerciseStore, roles auth.RoleQuerier, quota QuotaChecker) *Service {
	return &Service{store: store, roles: roles, quota: quota}
}

// viewer resolves catalog visibility: admins see everything, trainers see
// global plus their own private exercises.
func (s *Service) viewer(ctx context.Context, userID uuid.UUID) (Viewer, error) {
	isAdmin, err := auth.UserIsAdmin(ctx, s.roles, userID)
	if err != nil {
		return Viewer{}, err
	}
	if isAdmin {
		return Viewer{IncludeAll: true}, nil
	}
	trainerID, err := s.store.TrainerIDForUser(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Viewer{}, ErrForbidden
		}
		return Viewer{}, err
	}
	return Viewer{TrainerID: &trainerID}, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, params ListParams) (ListResult, error) {
	viewer, err := s.viewer(ctx, userID)
	if err != nil {
		return ListResult{}, err
	}
	return s.store.List(ctx, viewer, params)
}

func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (Exercise, error) {
	viewer, err := s.viewer(ctx, userID)
	if err != nil {
		return Exercise{}, err
	}
	ex, err := s.store.GetByID(ctx, id)
	if err != nil {
		return Exercise{}, err
	}
	if !visibleTo(viewer, ex) {
		return Exercise{}, pgx.ErrNoRows
	}
	return ex, nil
}

// Create makes a global exercise for admins and a private one for trainers.
// The trainer path enforces the exercise quota inside the store transaction.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in UpsertInput) (Exercise, error) {
	if err := in.Validate(); err != nil {
		return Exercise{}, err
	}
	viewer, err := s.viewer(ctx, userID)
	if err != nil {
		return Exercise{}, err
	}
	if viewer.IncludeAll {
		return s.store.Create(ctx, userID, nil, in)
	}
	return s.store.Create(ctx, userID, viewer.TrainerID, in)
}

// Update lets admins edit global exercises and trainers edit their own,
// blocking trainers while their exercise usage exceeds the plan limit.
func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, in UpsertInput) (Exercise, error) {
	if err := in.Validate(); err != nil {
		return Exercise{}, err
	}
	viewer, err := s.viewer(ctx, userID)
	if err != nil {
		return Exercise{}, err
	}
	if viewer.IncludeAll {
		return s.store.Update(ctx, id, userID, nil, in)
	}
	if err := s.checkQuota(ctx, userID, subscription.OpMutate); err != nil {
		return Exercise{}, err
	}
	return s.store.Update(ctx, id, userID, viewer.TrainerID, in)
}

// DeleteMany is always allowed within the caller's scope: deleting frees quota.
func (s *Service) DeleteMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (int64, error) {
	viewer, err := s.viewer(ctx, userID)
	if err != nil {
		return 0, err
	}
	if viewer.IncludeAll {
		return s.store.DeleteMany(ctx, userID, nil, ids)
	}
	return s.store.DeleteMany(ctx, userID, viewer.TrainerID, ids)
}

func (s *Service) checkQuota(ctx context.Context, userID uuid.UUID, op subscription.Op) error {
	if s.quota == nil {
		return nil
	}
	return s.quota.CheckQuota(ctx, userID, subscription.ResourceExercises, op)
}

func visibleTo(viewer Viewer, ex Exercise) bool {
	if viewer.IncludeAll || ex.Scope == ScopeGlobal {
		return true
	}
	owner := ex.OwnerTrainerID()
	return viewer.TrainerID != nil && owner != nil && *viewer.TrainerID == *owner
}
