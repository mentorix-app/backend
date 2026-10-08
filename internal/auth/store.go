package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

const (
	pgUniqueViolationCode     = "23505"
	pgForeignKeyViolationCode = "23503"
	pgCheckViolationCode      = "23514"
)

// isReferenceOrCheckViolation reports a foreign key or check violation: the
// database refuses a change that the rest of the data does not allow.
func isReferenceOrCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == pgForeignKeyViolationCode || pgErr.Code == pgCheckViolationCode)
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

// ClientIdentity describes a provider account that signs in as a client.
// Email is empty when the provider did not verify it.
type ClientIdentity struct {
	Provider    string
	Subject     string
	Email       string
	DisplayName string
}

// FindOrCreateClientByIdentity returns the user behind (Provider, Subject) and
// creates a client user with that identity when none exists. Concurrent first
// sign-ins end with one user.
func (s *Store) FindOrCreateClientByIdentity(ctx context.Context, id ClientIdentity) (uuid.UUID, error) {
	lookup := sqlc.GetAuthIdentityUserIDParams{Provider: id.Provider, Subject: id.Subject}
	existing, err := s.q.GetAuthIdentityUserID(ctx, lookup)
	if err == nil {
		return pgconv.FromPGUUID(existing), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("load identity: %w", err)
	}

	created, userID, err := s.createClientWithIdentity(ctx, id)
	if err != nil {
		return uuid.Nil, err
	}
	if created {
		return userID, nil
	}
	// A concurrent first sign-in won; its identity is committed now.
	existing, err = s.q.GetAuthIdentityUserID(ctx, lookup)
	if err != nil {
		return uuid.Nil, fmt.Errorf("load identity after conflict: %w", err)
	}
	return pgconv.FromPGUUID(existing), nil
}

// createClientWithIdentity reports false, with the transaction rolled back,
// when the identity already exists.
func (s *Store) createClientWithIdentity(ctx context.Context, id ClientIdentity) (bool, uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	var email *string
	if id.Email != "" {
		email = &id.Email
	}
	userPG, err := qtx.InsertUser(ctx, sqlc.InsertUserParams{
		PrimaryEmail: email,
		DisplayName:  id.DisplayName,
	})
	if err != nil {
		return false, uuid.Nil, fmt.Errorf("insert user: %w", err)
	}
	rows, err := qtx.InsertAuthIdentityIfAbsent(ctx, sqlc.InsertAuthIdentityIfAbsentParams{
		UserID:   userPG,
		Provider: id.Provider,
		Subject:  id.Subject,
	})
	if err != nil {
		return false, uuid.Nil, fmt.Errorf("insert auth identity: %w", err)
	}
	if rows == 0 {
		return false, uuid.Nil, nil
	}
	if err := qtx.InsertUserRole(ctx, sqlc.InsertUserRoleParams{
		UserID: userPG,
		Role:   RoleClient,
	}); err != nil {
		return false, uuid.Nil, fmt.Errorf("insert user role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return true, pgconv.FromPGUUID(userPG), nil
}

// LinkTelegram attaches telegramUserID to appUserID and returns the user that
// stays. A Telegram nobody holds is attached to the app account. A Telegram
// that already has an account absorbs the app account's Google and Apple
// sign-in and the app account is deleted, but only when the app account is
// empty: no trainer link and no role besides client. Otherwise
// ErrTelegramLinkConflict. ErrTelegramAlreadyLinked means the app account holds
// a different Telegram; pgx.ErrNoRows means the app account does not exist.
func (s *Store) LinkTelegram(ctx context.Context, appUserID uuid.UUID, telegramUserID string) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	appPG := pgconv.ToPGUUID(appUserID)

	if _, err := qtx.GetUserByID(ctx, appPG); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, pgx.ErrNoRows
		}
		return uuid.Nil, fmt.Errorf("load app user: %w", err)
	}

	own, err := qtx.GetTelegramSubjectByUserID(ctx, sqlc.GetTelegramSubjectByUserIDParams{
		UserID:   appPG,
		Provider: ProviderTelegram,
	})
	switch {
	case err == nil && own == telegramUserID:
		return appUserID, nil
	case err == nil:
		return uuid.Nil, ErrTelegramAlreadyLinked
	case !errors.Is(err, pgx.ErrNoRows):
		return uuid.Nil, fmt.Errorf("load app telegram: %w", err)
	}

	ownerPG, err := qtx.GetAuthIdentityUserID(ctx, sqlc.GetAuthIdentityUserIDParams{
		Provider: ProviderTelegram,
		Subject:  telegramUserID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		rows, err := qtx.InsertAuthIdentityIfAbsent(ctx, sqlc.InsertAuthIdentityIfAbsentParams{
			UserID:   appPG,
			Provider: ProviderTelegram,
			Subject:  telegramUserID,
		})
		if err != nil {
			return uuid.Nil, fmt.Errorf("insert telegram identity: %w", err)
		}
		if rows == 0 {
			// Lost a race: read the winner. The same app user twice (a double tap) is a success.
			winnerPG, err := qtx.GetAuthIdentityUserID(ctx, sqlc.GetAuthIdentityUserIDParams{
				Provider: ProviderTelegram,
				Subject:  telegramUserID,
			})
			if err != nil {
				return uuid.Nil, fmt.Errorf("reload telegram identity: %w", err)
			}
			if winnerPG != appPG {
				return uuid.Nil, ErrTelegramLinkConflict
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return uuid.Nil, fmt.Errorf("commit: %w", err)
		}
		return appUserID, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("load telegram identity: %w", err)
	}
	if ownerPG == appPG {
		return appUserID, nil
	}

	empty, err := appAccountIsEmpty(ctx, qtx, appPG)
	if err != nil {
		return uuid.Nil, err
	}
	if !empty {
		return uuid.Nil, ErrTelegramLinkConflict
	}

	if _, err := qtx.MoveSignInIdentitiesToUser(ctx, sqlc.MoveSignInIdentitiesToUserParams{
		ToUserID:   ownerPG,
		FromUserID: appPG,
		Providers:  []string{ProviderGoogle, ProviderApple},
	}); err != nil {
		return uuid.Nil, fmt.Errorf("move sign-in identities: %w", err)
	}
	if err := qtx.FillUserPrimaryEmailIfEmpty(ctx, sqlc.FillUserPrimaryEmailIfEmptyParams{
		UserID:     ownerPG,
		FromUserID: appPG,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("fill primary email: %w", err)
	}
	if err := qtx.GrantUserRole(ctx, sqlc.GrantUserRoleParams{UserID: ownerPG, Role: RoleClient}); err != nil {
		if isReferenceOrCheckViolation(err) {
			// The Telegram account is an admin, and an admin cannot hold the client role.
			return uuid.Nil, ErrTelegramLinkConflict
		}
		return uuid.Nil, fmt.Errorf("grant client role: %w", err)
	}
	if _, err := qtx.DeleteUserByID(ctx, appPG); err != nil {
		if isReferenceOrCheckViolation(err) {
			// Something still references the app user; the rollback undoes the identity move.
			return uuid.Nil, ErrTelegramLinkConflict
		}
		return uuid.Nil, fmt.Errorf("delete app user: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return pgconv.FromPGUUID(ownerPG), nil
}

// appAccountIsEmpty reports whether the user has no trainer link, no role
// besides client and no sign-in besides Google and Apple. Workouts and assignments cannot exist without a trainer link.
func appAccountIsEmpty(ctx context.Context, q *sqlc.Queries, userPG pgtype.UUID) (bool, error) {
	linked, err := q.UserHasTrainerLink(ctx, userPG)
	if err != nil {
		return false, fmt.Errorf("check trainer links: %w", err)
	}
	if linked {
		return false, nil
	}
	roles, err := q.ListUserRoles(ctx, userPG)
	if err != nil {
		return false, fmt.Errorf("load user roles: %w", err)
	}
	for _, role := range roles {
		if role != RoleClient {
			return false, nil
		}
	}
	others, err := q.CountUserIdentitiesOutsideProviders(ctx, sqlc.CountUserIdentitiesOutsideProvidersParams{
		UserID:    userPG,
		Providers: []string{ProviderGoogle, ProviderApple},
	})
	if err != nil {
		return false, fmt.Errorf("count other sign-ins: %w", err)
	}
	return others == 0, nil
}

type emailIdentityRow struct {
	UserID       uuid.UUID
	PasswordHash string
}

type UserProfile struct {
	Email     string
	Name      string
	CreatedAt time.Time
	Roles     []string
	// TelegramLinked is true when the account has a Telegram sign-in.
	TelegramLinked bool
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

	_, err = s.q.GetTelegramSubjectByUserID(ctx, sqlc.GetTelegramSubjectByUserIDParams{
		UserID:   pgconv.ToPGUUID(userID),
		Provider: ProviderTelegram,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return UserProfile{}, fmt.Errorf("load telegram identity: %w", err)
	}

	return UserProfile{
		Email:          row.PrimaryEmail,
		Name:           row.DisplayName,
		CreatedAt:      row.CreatedAt.UTC(),
		Roles:          roles,
		TelegramLinked: err == nil,
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

func (s *Store) RotateRefreshSession(ctx context.Context, oldHash, newHash []byte, newExpiresAt time.Time) (uuid.UUID, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	userPG, err := qtx.GetRefreshSessionUserForUpdate(ctx, oldHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", ErrInvalidRefresh
		}
		return uuid.Nil, "", fmt.Errorf("load refresh session: %w", err)
	}

	if err := qtx.RevokeRefreshSessionByHash(ctx, oldHash); err != nil {
		return uuid.Nil, "", fmt.Errorf("revoke refresh session: %w", err)
	}
	if err := qtx.InsertRefreshSession(ctx, sqlc.InsertRefreshSessionParams{
		UserID:    userPG,
		TokenHash: newHash,
		ExpiresAt: newExpiresAt,
	}); err != nil {
		return uuid.Nil, "", fmt.Errorf("insert rotated refresh session: %w", err)
	}
	user, err := qtx.GetUserByID(ctx, userPG)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("load user email: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, "", fmt.Errorf("commit: %w", err)
	}
	return pgconv.FromPGUUID(userPG), user.PrimaryEmail, nil
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
