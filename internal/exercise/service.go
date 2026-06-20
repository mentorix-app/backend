package exercise

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	store *Store
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
