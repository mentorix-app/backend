package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	store     *Store
	jwtSecret []byte
}

func NewService(pool *pgxpool.Pool, jwtSecret string) *Service {
	return &Service{
		store:     NewStore(pool),
		jwtSecret: []byte(jwtSecret),
	}
}

func (s *Service) RegisterTrainer(ctx context.Context, email, password string) (accessToken string, expiresAt time.Time, userID uuid.UUID, err error) {
	if err := ValidatePassword(password); err != nil {
		return "", time.Time{}, uuid.Nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", time.Time{}, uuid.Nil, fmt.Errorf("hash password: %w", err)
	}
	userID, err = s.store.RegisterTrainerEmailPassword(ctx, email, hash)
	if err != nil {
		return "", time.Time{}, uuid.Nil, err
	}
	token, exp, err := signAccessToken(userID, s.jwtSecret)
	if err != nil {
		return "", time.Time{}, uuid.Nil, fmt.Errorf("sign token: %w", err)
	}
	return token, exp, userID, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (accessToken string, expiresAt time.Time, userID uuid.UUID, err error) {
	row, err := s.store.getEmailPasswordIdentity(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", time.Time{}, uuid.Nil, ErrInvalidCredentials
		}
		return "", time.Time{}, uuid.Nil, fmt.Errorf("load identity: %w", err)
	}
	if row.PasswordHash == "" {
		return "", time.Time{}, uuid.Nil, ErrInvalidCredentials
	}
	ok, err := PasswordMatches(row.PasswordHash, password)
	if err != nil {
		return "", time.Time{}, uuid.Nil, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return "", time.Time{}, uuid.Nil, ErrInvalidCredentials
	}
	token, exp, err := signAccessToken(row.UserID, s.jwtSecret)
	if err != nil {
		return "", time.Time{}, uuid.Nil, fmt.Errorf("sign token: %w", err)
	}
	return token, exp, row.UserID, nil
}

func (s *Service) UserPrimaryEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	return s.store.UserPrimaryEmail(ctx, userID)
}
