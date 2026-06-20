package exercise

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type exerciseStore interface {
	List(ctx context.Context, params ListParams) (ListResult, error)
	GetByID(ctx context.Context, id uuid.UUID) (Exercise, error)
	Create(ctx context.Context, userID uuid.UUID, in UpsertInput) (Exercise, error)
	Update(ctx context.Context, id, userID uuid.UUID, in UpsertInput) (Exercise, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteAll(ctx context.Context) (int64, error)
}

type Service struct {
	store exerciseStore
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{store: NewStore(pool)}
}

func (s *Service) List(ctx context.Context, params ListParams) (ListResult, error) {
	return s.store.List(ctx, params)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Exercise, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in UpsertInput) (Exercise, error) {
	if err := in.Validate(); err != nil {
		return Exercise{}, err
	}
	return s.store.Create(ctx, userID, in)
}

func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, in UpsertInput) (Exercise, error) {
	if err := in.Validate(); err != nil {
		return Exercise{}, err
	}
	return s.store.Update(ctx, id, userID, in)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.store.Delete(ctx, id)
}

func (s *Service) DeleteAll(ctx context.Context) (int64, error) {
	return s.store.DeleteAll(ctx)
}
