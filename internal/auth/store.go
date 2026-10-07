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

const (
	pgUniqueViolationCode = "23505"
	// pgCheckViolationCode is what the user_roles_admin_exclusive trigger raises.
	pgCheckViolationCode = "23514"
	// pgForeignKeyViolationCode is raised when a row refers to a missing user.
	pgForeignKeyViolationCode = "23503"

	usersEmailUniqueConstraint    = "users_primary_email_lower_uniq"
	identitySubjectUniqConstraint = "auth_identities_provider_subject_uniq"
)

// isUniqueViolation reports whether err is a unique violation of the named
// constraint, so one violation cannot be mistaken for another.
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolationCode && pgErr.ConstraintName == constraint
}

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
		if isUniqueViolation(err, usersEmailUniqueConstraint) {
			return uuid.Nil, ErrEmailTaken
		}
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

// SocialSignIn returns the user that owns the (provider, subject) identity and
// creates the user and identity when none exists. A new user has no roles. The
// claims' email is stored as primary_email only when the provider marked it
// verified. A verified email that already belongs to another user fails with
// ErrEmailBelongsToAnotherAccount and creates nothing.
func (s *Store) SocialSignIn(ctx context.Context, provider string, claims IDTokenClaims, displayName string) (uuid.UUID, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	identity := sqlc.GetAuthIdentityUserIDParams{Provider: provider, Subject: claims.Subject}

	userPG, err := qtx.GetAuthIdentityUserID(ctx, identity)
	if err == nil {
		return pgconv.FromPGUUID(userPG), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, fmt.Errorf("lookup %s identity: %w", provider, err)
	}

	var primaryEmail *string
	if email := NormalizeEmail(claims.Email); claims.EmailVerified && email != "" {
		_, err := qtx.GetUserIDByPrimaryEmail(ctx, email)
		if err == nil {
			return uuid.Nil, false, ErrEmailBelongsToAnotherAccount
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, fmt.Errorf("lookup user by email: %w", err)
		}
		primaryEmail = &email
	}

	userPG, err = qtx.InsertUser(ctx, sqlc.InsertUserParams{
		PrimaryEmail: primaryEmail,
		DisplayName:  displayName,
	})
	if err != nil {
		if isUniqueViolation(err, usersEmailUniqueConstraint) {
			// Either another account holds the email, or a concurrent first
			// sign-in of this same identity just created its user with it. The
			// failed insert aborted the transaction; read the identity outside it.
			_ = tx.Rollback(ctx)
			existing, lookupErr := s.q.GetAuthIdentityUserID(ctx, identity)
			if lookupErr == nil {
				return pgconv.FromPGUUID(existing), false, nil
			}
			if !errors.Is(lookupErr, pgx.ErrNoRows) {
				return uuid.Nil, false, fmt.Errorf("lookup %s identity after email conflict: %w", provider, lookupErr)
			}
			return uuid.Nil, false, ErrEmailBelongsToAnotherAccount
		}
		return uuid.Nil, false, fmt.Errorf("insert user: %w", err)
	}
	if err := qtx.InsertAuthIdentity(ctx, sqlc.InsertAuthIdentityParams{
		UserID:   userPG,
		Provider: provider,
		Subject:  claims.Subject,
	}); err != nil {
		if isUniqueViolation(err, identitySubjectUniqConstraint) {
			// A concurrent first sign-in won. The failed insert aborted this
			// transaction, so roll it back (dropping the new user) and read the
			// winner's identity outside it.
			_ = tx.Rollback(ctx)
			userPG, err = s.q.GetAuthIdentityUserID(ctx, identity)
			if err != nil {
				return uuid.Nil, false, fmt.Errorf("lookup %s identity after race: %w", provider, err)
			}
			return pgconv.FromPGUUID(userPG), false, nil
		}
		return uuid.Nil, false, fmt.Errorf("insert %s identity: %w", provider, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, false, fmt.Errorf("commit: %w", err)
	}
	return pgconv.FromPGUUID(userPG), true, nil
}

// AddRole gives the user a self-assignable role (client or trainer) and is a
// no-op when the user already has it. The trainer role also creates the
// trainers row. An admin account fails with ErrRoleConflict.
func (s *Store) AddRole(ctx context.Context, userID uuid.UUID, role string) error {
	if role != RoleClient && role != RoleTrainer {
		return fmt.Errorf("role %q cannot be added by the user", role)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	userPG := pgconv.ToPGUUID(userID)

	if err := qtx.GrantUserRole(ctx, sqlc.GrantUserRoleParams{UserID: userPG, Role: role}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgCheckViolationCode {
			return ErrRoleConflict
		}
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolationCode {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("grant role: %w", err)
	}
	if role == RoleTrainer {
		if err := qtx.InsertTrainerIfMissing(ctx, userPG); err != nil {
			return fmt.Errorf("insert trainer: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
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
