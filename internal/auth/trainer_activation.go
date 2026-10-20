package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

// ActivateTrainer gives the user the trainer role next to the roles they already hold, creates
// their trainers row when they have none, and records that they accepted the trainer terms
// termsVersion. All of it happens in one transaction and is safe to repeat: the same version
// again changes nothing and keeps the time of the first acceptance, another version replaces
// the version and the time.
//
// The trainers row is locked before the user row, the order the quota writers use, and the
// roles are read under the user lock. ErrAdminCannotActivate means the user is an admin, which
// cannot hold the trainer role; nothing is changed then. pgx.ErrNoRows means the user does not
// exist.
func (s *Store) ActivateTrainer(ctx context.Context, userID uuid.UUID, termsVersion string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	userPG := pgconv.ToPGUUID(userID)

	if _, err := qtx.LockTrainerByUserID(ctx, userPG); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock trainer: %w", err)
	}
	if _, err := qtx.LockUserForUpdate(ctx, userPG); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("lock user: %w", err)
	}

	roles, err := qtx.ListUserRoles(ctx, userPG)
	if err != nil {
		return fmt.Errorf("load user roles: %w", err)
	}
	if slices.Contains(roles, RoleAdmin) {
		return ErrAdminCannotActivate
	}

	if err := qtx.GrantUserRole(ctx, sqlc.GrantUserRoleParams{UserID: userPG, Role: RoleTrainer}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgCheckViolationCode {
			// The admin exclusivity trigger: an admin role arrived after the check above.
			return ErrAdminCannotActivate
		}
		return fmt.Errorf("grant trainer role: %w", err)
	}
	if err := qtx.InsertTrainerIfAbsent(ctx, userPG); err != nil {
		return fmt.Errorf("insert trainer: %w", err)
	}
	if err := qtx.AcceptTrainerTerms(ctx, sqlc.AcceptTrainerTermsParams{TermsVersion: termsVersion, UserID: userPG}); err != nil {
		return fmt.Errorf("record trainer terms: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ActivateTrainer makes the user a trainer, see Store.ActivateTrainer, and returns their
// profile as it is afterwards.
func (s *Service) ActivateTrainer(ctx context.Context, userID uuid.UUID, termsVersion string) (UserProfile, error) {
	if err := s.store.ActivateTrainer(ctx, userID, termsVersion); err != nil {
		return UserProfile{}, err
	}
	return s.store.UserProfile(ctx, userID)
}

// normalizeTermsVersion trims surrounding white space and accepts only a real calendar date in
// exactly the form YYYY-MM-DD, so "2026-10-1" and "2026-02-30" are refused.
func normalizeTermsVersion(raw string) (string, bool) {
	version := strings.TrimSpace(raw)
	date, err := time.Parse(time.DateOnly, version)
	if err != nil || date.Format(time.DateOnly) != version {
		return "", false
	}
	return version, true
}
