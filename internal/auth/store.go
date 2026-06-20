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

	"mentorix-backend/internal/db"
)

const pgUniqueViolationCode = "23505"

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) RegisterTrainerEmailPassword(ctx context.Context, email, passwordHash string) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO `+db.Table("users")+` (primary_email) VALUES ($1) RETURNING id`, email).Scan(&userID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert user: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO `+db.Table("auth_identities")+` (user_id, provider, subject, password_hash) VALUES ($1, $2, $3, $4)`,
		userID, ProviderEmailPassword, email, passwordHash,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolationCode {
			return uuid.Nil, ErrEmailTaken
		}
		return uuid.Nil, fmt.Errorf("insert auth identity: %w", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO `+db.Table("user_roles")+` (user_id, role) VALUES ($1, $2)`, userID, RoleTrainer)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert user role: %w", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO `+db.Table("trainers")+` (user_id) VALUES ($1)`, userID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert trainer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return userID, nil
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
	CreatedAt time.Time
	Roles     []string
}

func (s *Store) UserProfile(ctx context.Context, userID uuid.UUID) (UserProfile, error) {
	var profile UserProfile
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(primary_email, ''), created_at FROM `+db.Table("users")+` WHERE id = $1`,
		userID,
	).Scan(&profile.Email, &profile.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserProfile{}, pgx.ErrNoRows
		}
		return UserProfile{}, fmt.Errorf("load user: %w", err)
	}
	profile.CreatedAt = profile.CreatedAt.UTC()

	roles, err := s.UserRoles(ctx, userID)
	if err != nil {
		return UserProfile{}, err
	}
	profile.Roles = roles
	return profile, nil
}

func (s *Store) UserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT role FROM `+db.Table("user_roles")+` WHERE user_id = $1 ORDER BY role`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("load user roles: %w", err)
	}
	defer rows.Close()

	roles := make([]string, 0)
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user roles: %w", err)
	}
	return roles, nil
}

func (s *Store) GrantRole(ctx context.Context, userID uuid.UUID, role string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO `+db.Table("user_roles")+` (user_id, role) VALUES ($1, $2) ON CONFLICT (user_id, role) DO NOTHING`,
		userID, role,
	)
	if err != nil {
		return fmt.Errorf("grant role: %w", err)
	}
	return nil
}

func (s *Store) getEmailPasswordIdentity(ctx context.Context, email string) (emailIdentityRow, error) {
	var row emailIdentityRow
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, COALESCE(password_hash, '') FROM `+db.Table("auth_identities")+` WHERE provider = $1 AND subject = $2`,
		ProviderEmailPassword, email,
	).Scan(&row.UserID, &row.PasswordHash)
	return row, err
}

func (s *Store) InsertRefreshSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO `+db.Table("refresh_sessions")+` (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	if err != nil {
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

	var userID uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT user_id FROM `+db.Table("refresh_sessions")+` WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now() FOR UPDATE`,
		oldHash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrInvalidRefresh
		}
		return uuid.Nil, fmt.Errorf("load refresh session: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE `+db.Table("refresh_sessions")+` SET revoked_at = now() WHERE token_hash = $1`, oldHash); err != nil {
		return uuid.Nil, fmt.Errorf("revoke refresh session: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO `+db.Table("refresh_sessions")+` (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, newHash, newExpiresAt,
	); err != nil {
		return uuid.Nil, fmt.Errorf("insert rotated refresh session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return userID, nil
}

func (s *Store) RevokeRefreshSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE `+db.Table("refresh_sessions")+` SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	return nil
}

func (s *Store) RevokeAllUserRefreshSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE `+db.Table("refresh_sessions")+` SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("revoke all refresh sessions: %w", err)
	}
	return nil
}
