package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
	err = tx.QueryRow(ctx, `INSERT INTO mentorix.users (primary_email) VALUES ($1) RETURNING id`, email).Scan(&userID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert user: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO mentorix.auth_identities (user_id, provider, subject, password_hash) VALUES ($1, $2, $3, $4)`,
		userID, ProviderEmailPassword, email, passwordHash,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return uuid.Nil, ErrEmailTaken
		}
		return uuid.Nil, fmt.Errorf("insert auth identity: %w", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO mentorix.user_roles (user_id, role) VALUES ($1, 'trainer')`, userID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert user role: %w", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO mentorix.trainers (user_id) VALUES ($1)`, userID)
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
	var email string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(primary_email, '') FROM mentorix.users WHERE id = $1`, userID).Scan(&email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", pgx.ErrNoRows
		}
		return "", fmt.Errorf("load user: %w", err)
	}
	return email, nil
}

func (s *Store) getEmailPasswordIdentity(ctx context.Context, email string) (emailIdentityRow, error) {
	var row emailIdentityRow
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, COALESCE(password_hash, '') FROM mentorix.auth_identities WHERE provider = $1 AND subject = $2`,
		ProviderEmailPassword, email,
	).Scan(&row.UserID, &row.PasswordHash)
	return row, err
}
