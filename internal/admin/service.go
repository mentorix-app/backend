package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

type roleStore interface {
	UserProfile(ctx context.Context, userID uuid.UUID) (auth.UserProfile, error)
	GrantRole(ctx context.Context, userID uuid.UUID, role string) error
}

type Service struct {
	store roleStore
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{store: auth.NewStore(pool)}
}

func (s *Service) GrantAdmin(ctx context.Context, targetUserID uuid.UUID) (auth.UserProfile, error) {
	if _, err := s.store.UserProfile(ctx, targetUserID); err != nil {
		return auth.UserProfile{}, err
	}
	if err := s.store.GrantRole(ctx, targetUserID, auth.RoleAdmin); err != nil {
		return auth.UserProfile{}, fmt.Errorf("grant admin: %w", err)
	}
	profile, err := s.store.UserProfile(ctx, targetUserID)
	if err != nil {
		return auth.UserProfile{}, fmt.Errorf("load profile: %w", err)
	}
	return profile, nil
}
