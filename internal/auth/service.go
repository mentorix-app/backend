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
	store      *Store
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type IssuedAuth struct {
	AccessToken   string
	AccessExpires time.Time
	RefreshToken  string
	UserID        uuid.UUID
	Email         string
}

func NewService(pool *pgxpool.Pool, jwtSecret string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{
		store:      NewStore(pool),
		jwtSecret:  []byte(jwtSecret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (s *Service) RegisterTrainer(ctx context.Context, email, password string) (IssuedAuth, error) {
	var out IssuedAuth
	if err := ValidatePassword(password); err != nil {
		return out, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return out, fmt.Errorf("hash password: %w", err)
	}
	userID, err := s.store.RegisterTrainerEmailPassword(ctx, email, hash)
	if err != nil {
		return out, err
	}
	plain, h, err := newRefreshToken()
	if err != nil {
		return out, fmt.Errorf("refresh token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.refreshTTL)
	if err := s.store.InsertRefreshSession(ctx, userID, h, expiresAt); err != nil {
		return out, fmt.Errorf("session: %w", err)
	}
	token, exp, err := signAccessToken(userID, s.jwtSecret, s.accessTTL)
	if err != nil {
		return out, fmt.Errorf("sign token: %w", err)
	}
	out.AccessToken = token
	out.AccessExpires = exp
	out.RefreshToken = plain
	out.UserID = userID
	out.Email = email
	return out, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (IssuedAuth, error) {
	var out IssuedAuth
	row, err := s.store.getEmailPasswordIdentity(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, ErrInvalidCredentials
		}
		return out, fmt.Errorf("load identity: %w", err)
	}
	if row.PasswordHash == "" {
		return out, ErrInvalidCredentials
	}
	ok, err := PasswordMatches(row.PasswordHash, password)
	if err != nil {
		return out, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return out, ErrInvalidCredentials
	}
	plain, h, err := newRefreshToken()
	if err != nil {
		return out, fmt.Errorf("refresh token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.refreshTTL)
	if err := s.store.InsertRefreshSession(ctx, row.UserID, h, expiresAt); err != nil {
		return out, fmt.Errorf("session: %w", err)
	}
	token, exp, err := signAccessToken(row.UserID, s.jwtSecret, s.accessTTL)
	if err != nil {
		return out, fmt.Errorf("sign token: %w", err)
	}
	out.AccessToken = token
	out.AccessExpires = exp
	out.RefreshToken = plain
	out.UserID = row.UserID
	out.Email = email
	return out, nil
}

func (s *Service) Refresh(ctx context.Context, refreshPlain string) (IssuedAuth, error) {
	var out IssuedAuth
	if refreshPlain == "" {
		return out, ErrInvalidRefresh
	}
	oldHash := hashRefreshToken(refreshPlain)
	newPlain, newHash, err := newRefreshToken()
	if err != nil {
		return out, fmt.Errorf("refresh token: %w", err)
	}
	newExpires := time.Now().UTC().Add(s.refreshTTL)
	userID, err := s.store.RotateRefreshSession(ctx, oldHash, newHash, newExpires)
	if err != nil {
		return out, err
	}
	email, err := s.store.UserPrimaryEmail(ctx, userID)
	if err != nil {
		return out, fmt.Errorf("load user email: %w", err)
	}
	token, exp, err := signAccessToken(userID, s.jwtSecret, s.accessTTL)
	if err != nil {
		return out, fmt.Errorf("sign token: %w", err)
	}
	out.AccessToken = token
	out.AccessExpires = exp
	out.RefreshToken = newPlain
	out.UserID = userID
	out.Email = email
	return out, nil
}

func (s *Service) Logout(ctx context.Context, refreshPlain string) error {
	if refreshPlain == "" {
		return nil
	}
	return s.store.RevokeRefreshSession(ctx, hashRefreshToken(refreshPlain))
}

func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	return s.store.RevokeAllUserRefreshSessions(ctx, userID)
}

func (s *Service) UserPrimaryEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	return s.store.UserPrimaryEmail(ctx, userID)
}

func (s *Service) UserProfile(ctx context.Context, userID uuid.UUID) (UserProfile, error) {
	return s.store.UserProfile(ctx, userID)
}
