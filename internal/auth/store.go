package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

const pgUniqueViolationCode = "23505"

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

func (s *Store) RegisterTrainerEmailPassword(ctx context.Context, email, passwordHash, displayName string) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	userPG, err := qtx.InsertUser(ctx, sqlc.InsertUserParams{
		PrimaryEmail: &email,
		DisplayName:  displayName,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert user: %w", err)
	}

	if err := qtx.InsertAuthIdentity(ctx, sqlc.InsertAuthIdentityParams{
		UserID:       userPG,
		Provider:     ProviderEmailPassword,
		Subject:      email,
		PasswordHash: &passwordHash,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolationCode {
			return uuid.Nil, ErrEmailTaken
		}
		return uuid.Nil, fmt.Errorf("insert auth identity: %w", err)
	}

	if err := qtx.InsertUserRole(ctx, sqlc.InsertUserRoleParams{
		UserID: userPG,
		Role:   RoleTrainer,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("insert user role: %w", err)
	}

	if err := qtx.InsertTrainer(ctx, userPG); err != nil {
		return uuid.Nil, fmt.Errorf("insert trainer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return pgconv.FromPGUUID(userPG), nil
}

type emailIdentityRow struct {
	UserID       uuid.UUID
	PasswordHash string
}

func (s *Store) UserPrimaryEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	profile, err := s.UserProfile(ctx, userID)
	if err != nil {
		return "", err
	}
	return profile.Email, nil
}

type UserProfile struct {
	Email     string
	Name      string
	CreatedAt time.Time
	Roles     []string
}

func (s *Store) UserProfile(ctx context.Context, userID uuid.UUID) (UserProfile, error) {
	row, err := s.q.GetUserByID(ctx, pgconv.ToPGUUID(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserProfile{}, pgx.ErrNoRows
		}
		return UserProfile{}, fmt.Errorf("load user: %w", err)
	}

	roles, err := s.UserRoles(ctx, userID)
	if err != nil {
		return UserProfile{}, err
	}

	return UserProfile{
		Email:     row.PrimaryEmail,
		Name:      row.DisplayName,
		CreatedAt: row.CreatedAt.UTC(),
		Roles:     roles,
	}, nil
}

func (s *Store) UpdateUserDisplayName(ctx context.Context, userID uuid.UUID, displayName string) error {
	rows, err := s.q.UpdateUserDisplayName(ctx, sqlc.UpdateUserDisplayNameParams{
		ID:          pgconv.ToPGUUID(userID),
		DisplayName: displayName,
	})
	if err != nil {
		return fmt.Errorf("update display name: %w", err)
	}
	if rows == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	roles, err := s.q.ListUserRoles(ctx, pgconv.ToPGUUID(userID))
	if err != nil {
		return nil, fmt.Errorf("load user roles: %w", err)
	}
	return roles, nil
}

func (s *Store) GrantRole(ctx context.Context, userID uuid.UUID, role string) error {
	if err := s.q.GrantUserRole(ctx, sqlc.GrantUserRoleParams{
		UserID: pgconv.ToPGUUID(userID),
		Role:   role,
	}); err != nil {
		return fmt.Errorf("grant role: %w", err)
	}
	return nil
}

func (s *Store) getEmailPasswordIdentity(ctx context.Context, email string) (emailIdentityRow, error) {
	row, err := s.q.GetEmailPasswordIdentity(ctx, sqlc.GetEmailPasswordIdentityParams{
		Provider: ProviderEmailPassword,
		Subject:  email,
	})
	if err != nil {
		return emailIdentityRow{}, err
	}
	return emailIdentityRow{
		UserID:       pgconv.FromPGUUID(row.UserID),
		PasswordHash: row.PasswordHash,
	}, nil
}

func (s *Store) InsertRefreshSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	if err := s.q.InsertRefreshSession(ctx, sqlc.InsertRefreshSessionParams{
		UserID:    pgconv.ToPGUUID(userID),
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("insert refresh session: %w", err)
	}
	return nil
}

func (s *Store) RotateRefreshSession(ctx context.Context, oldHash, newHash []byte, newExpiresAt time.Time) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	userPG, err := qtx.GetRefreshSessionUserForUpdate(ctx, oldHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrInvalidRefresh
		}
		return uuid.Nil, fmt.Errorf("load refresh session: %w", err)
	}

	if err := qtx.RevokeRefreshSessionByHash(ctx, oldHash); err != nil {
		return uuid.Nil, fmt.Errorf("revoke refresh session: %w", err)
	}
	if err := qtx.InsertRefreshSession(ctx, sqlc.InsertRefreshSessionParams{
		UserID:    userPG,
		TokenHash: newHash,
		ExpiresAt: newExpiresAt,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("insert rotated refresh session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return pgconv.FromPGUUID(userPG), nil
}

func (s *Store) RevokeRefreshSession(ctx context.Context, tokenHash []byte) error {
	if err := s.q.RevokeRefreshSessionByHash(ctx, tokenHash); err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	return nil
}

func (s *Store) RevokeAllUserRefreshSessions(ctx context.Context, userID uuid.UUID) error {
	if err := s.q.RevokeAllUserRefreshSessions(ctx, pgconv.ToPGUUID(userID)); err != nil {
		return fmt.Errorf("revoke all refresh sessions: %w", err)
	}
	return nil
}
