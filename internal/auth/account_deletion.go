package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/db/pgconv"
)

// telegramCleanupTimeout bounds the cleanup that runs after the commit.
const telegramCleanupTimeout = 5 * time.Second

// DeleteClientAccount deletes a client and every row that belongs to them in one
// transaction, including the invites they consumed, and returns their Telegram
// user id, empty when there is none.
//
// The user row is locked first, so an assignment committed at the same moment is
// either seen and deleted too or fails on a user that is gone. Both refusals are
// decided under that lock, and nothing is changed when one applies:
// ErrAccountDeletionUnavailable when the roles are not exactly client, then
// ErrAppleSignInNotRevoked when the account has an Apple sign-in.
// ErrAccountDeletionUnavailable is also returned when the database refuses a delete
// because other rows still reference the user. pgx.ErrNoRows means the user does
// not exist.
func (s *Store) DeleteClientAccount(ctx context.Context, userID uuid.UUID) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	userPG := pgconv.ToPGUUID(userID)

	if _, err := qtx.LockUserForUpdate(ctx, userPG); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", pgx.ErrNoRows
		}
		return "", fmt.Errorf("lock user: %w", err)
	}

	roles, err := qtx.ListUserRoles(ctx, userPG)
	if err != nil {
		return "", fmt.Errorf("load user roles: %w", err)
	}
	if !onlyClientRole(roles) {
		return "", ErrAccountDeletionUnavailable
	}

	identities, err := qtx.ListAuthIdentitiesByUserID(ctx, userPG)
	if err != nil {
		return "", fmt.Errorf("load identities: %w", err)
	}
	var telegramUserID string
	for _, identity := range identities {
		switch identity.Provider {
		case ProviderApple:
			return "", ErrAppleSignInNotRevoked
		case ProviderTelegram:
			telegramUserID = identity.Subject
		}
	}

	// Assignments go before completions: a trainer who removes an assignment takes
	// the rows in this order (the foreign key action clears the completions' link),
	// so the two transactions never lock them the other way round.
	if _, err := qtx.DeleteProgramAssignmentsByClient(ctx, userPG); err != nil {
		return "", deletionError("delete program assignments", err)
	}
	if _, err := qtx.DeleteClientWorkoutCompletionsByClient(ctx, userPG); err != nil {
		return "", deletionError("delete workout completions", err)
	}
	if _, err := qtx.DeleteTrainerInvitesConsumedByUser(ctx, userPG); err != nil {
		return "", deletionError("delete consumed invites", err)
	}
	if _, err := qtx.DeleteUserByID(ctx, userPG); err != nil {
		return "", deletionError("delete user", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return telegramUserID, nil
}

// deletionError maps a database refusal to ErrAccountDeletionUnavailable and keeps
// the database error in the chain for the log.
func deletionError(what string, err error) error {
	if isReferenceOrCheckViolation(err) {
		return fmt.Errorf("%s: %w: %w", what, ErrAccountDeletionUnavailable, err)
	}
	return fmt.Errorf("%s: %w", what, err)
}

// onlyClientRole reports whether the roles are exactly client. An account with no
// role row is not deletable here either.
func onlyClientRole(roles []string) bool {
	return len(roles) == 1 && roles[0] == RoleClient
}

// DeleteAccount deletes the client's account and everything that belongs to it. The
// store decides who may be deleted, under a lock. ErrAccountDeletionUnavailable
// means the account is not deletable here, ErrProviderNotConfigured that it has an
// Apple sign-in and the Apple revocation is not configured (nothing is deleted), and
// pgx.ErrNoRows that the account does not exist.
func (s *Service) DeleteAccount(ctx context.Context, userID uuid.UUID) error {
	telegramUserID, err := s.store.DeleteClientAccount(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrAppleSignInNotRevoked) {
			return ErrProviderNotConfigured
		}
		return fmt.Errorf("delete account: %w", err)
	}

	if telegramUserID != "" && s.telegramCleanup != nil {
		// A client that drops the connection right after the commit must not cancel it.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), telegramCleanupTimeout)
		defer cancel()
		if err := s.telegramCleanup(cleanupCtx, telegramUserID); err != nil {
			s.log.Warn("telegram cleanup after account deletion failed", "user_id", userID, "error", err)
		}
	}
	s.log.Info("account deleted", "user_id", userID)
	return nil
}
