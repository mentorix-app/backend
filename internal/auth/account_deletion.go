package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/db/pgconv"
)

// telegramCleanupTimeout bounds the cleanup that runs after the commit.
const telegramCleanupTimeout = 5 * time.Second

// deleteAfterRevocationTimeout bounds the delete that follows a revocation at Apple.
const deleteAfterRevocationTimeout = 10 * time.Second

// DeleteClientAccount deletes a client and every row that belongs to them in one
// transaction, including the invites they consumed, and returns their Telegram
// user id, empty when there is none.
//
// revokedAppleSubject is the subject of the Apple sign-in whose authorization was
// revoked at Apple just before, empty when none was. An account that has Apple
// sign-ins is deleted only when that subject is one of them, so one revoked sign-in
// is enough for an account with two (a known limit).
//
// The user row is locked first, so an assignment committed at the same moment is
// either seen and deleted too or fails on a user that is gone. Both refusals are
// decided under that lock, and nothing is changed when one applies:
// ErrAccountDeletionUnavailable when the roles are not exactly client, then
// ErrAppleSignInNotRevoked when the account has an Apple sign-in and
// revokedAppleSubject is not one of them. ErrAccountDeletionUnavailable is also
// returned when the database refuses a delete because other rows still reference
// the user. pgx.ErrNoRows means the user does not exist.
func (s *Store) DeleteClientAccount(ctx context.Context, userID uuid.UUID, revokedAppleSubject string) (string, error) {
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
	var appleSubjects []string
	for _, identity := range identities {
		switch identity.Provider {
		case ProviderApple:
			appleSubjects = append(appleSubjects, identity.Subject)
		case ProviderTelegram:
			telegramUserID = identity.Subject
		}
	}
	if len(appleSubjects) > 0 && (revokedAppleSubject == "" || !slices.Contains(appleSubjects, revokedAppleSubject)) {
		return "", ErrAppleSignInNotRevoked
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

func (s *Service) canRevokeApple() bool {
	return s.appleRevoker != nil && s.appleVerifier != nil
}

// revokeAppleSignIn revokes the Apple authorization of an account that has an Apple
// sign-in and returns the subject that was revoked. It returns "" without calling
// Apple when the account has none.
//
// The role rule also runs here, ahead of the store, which still decides it under its
// lock: a revocation cannot be undone, so nothing is sent to Apple for an account that
// the store is about to refuse. The other refusals stay in the store.
func (s *Service) revokeAppleSignIn(ctx context.Context, userID uuid.UUID, proof AppleProof) (string, error) {
	subjects, err := s.store.UserAppleSubjects(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("load apple sign-ins: %w", err)
	}
	if len(subjects) == 0 {
		return "", nil
	}
	roles, err := s.store.UserRoles(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("load user roles: %w", err)
	}
	if !onlyClientRole(roles) {
		return "", ErrAccountDeletionUnavailable
	}
	if !s.canRevokeApple() {
		return "", ErrProviderNotConfigured
	}
	if proof.IDToken == "" || proof.AuthorizationCode == "" {
		return "", ErrAppleAuthorizationRequired
	}
	claims, err := s.appleVerifier.Verify(ctx, proof.IDToken)
	if err != nil {
		return "", err
	}
	if !slices.Contains(subjects, claims.Subject) {
		// Fixed words that name no subject: the log tells this from a code of another account.
		return "", fmt.Errorf("%w: the id_token belongs to an apple account that is not a sign-in of this account", ErrAppleAccountMismatch)
	}
	if err := s.appleRevoker.Revoke(ctx, claims.Audience, claims.Subject, proof.AuthorizationCode, proof.RedirectURI); err != nil {
		return "", fmt.Errorf("revoke apple authorization: %w", err)
	}
	return claims.Subject, nil
}

// UserAppleSubjects lists the subjects of the account's Apple sign-ins.
func (s *Store) UserAppleSubjects(ctx context.Context, userID uuid.UUID) ([]string, error) {
	identities, err := s.q.ListAuthIdentitiesByUserID(ctx, pgconv.ToPGUUID(userID))
	if err != nil {
		return nil, fmt.Errorf("load identities: %w", err)
	}
	var subjects []string
	for _, identity := range identities {
		if identity.Provider == ProviderApple {
			subjects = append(subjects, identity.Subject)
		}
	}
	return subjects, nil
}

// AppleProof is what the app sends with a deletion to revoke the account's Sign in with
// Apple authorization: a fresh ID token and authorization code, and the redirect address
// of the authorization request when it had one.
type AppleProof struct {
	IDToken           string
	AuthorizationCode string
	RedirectURI       string
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

// DeleteAccount deletes the client's account and everything that belongs to it.
//
// An account with an Apple sign-in is first revoked at Apple. Nothing is sent to Apple
// until five checks have passed, in this order: the roles are exactly client
// (ErrAccountDeletionUnavailable), the revocation is configured
// (ErrProviderNotConfigured), proof carries a token and a code
// (ErrAppleAuthorizationRequired), the token verifies (ErrInvalidIDToken), the token
// belongs to one of the account's Apple sign-ins (ErrAppleAccountMismatch). Then
// Apple revokes, which can end in ErrInvalidAppleAuthorizationCode,
// ErrAppleAccountMismatch (the code was issued to another Apple account than the
// token's) or ErrAppleRevocationFailed, and only then does the store delete. A failed revocation
// deletes nothing; a database failure after a successful one leaves the account, and
// the person signs in with Apple again and repeats. After a revocation the delete runs
// on a context that a dropped connection cannot cancel. proof is ignored for an
// account without an Apple sign-in.
//
// The store decides who may be deleted, under a lock. An Apple sign-in that arrived
// after the checks above is answered with ErrAppleAuthorizationRequired, or with
// ErrProviderNotConfigured when the revocation is not configured. pgx.ErrNoRows
// means the account does not exist.
func (s *Service) DeleteAccount(ctx context.Context, userID uuid.UUID, proof AppleProof) error {
	revokedSubject, err := s.revokeAppleSignIn(ctx, userID, proof)
	if err != nil {
		return err
	}
	deleteCtx := ctx
	if revokedSubject != "" {
		// Apple has revoked and cannot be asked to undo it. A cancelled transaction rolls back
		// completely, so a client that drops the connection now would leave the account in
		// place after its Apple authorization is gone.
		var cancel context.CancelFunc
		deleteCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), deleteAfterRevocationTimeout)
		defer cancel()
	}
	telegramUserID, err := s.store.DeleteClientAccount(deleteCtx, userID, revokedSubject)
	if err != nil {
		if errors.Is(err, ErrAppleSignInNotRevoked) {
			if !s.canRevokeApple() {
				return ErrProviderNotConfigured
			}
			return ErrAppleAuthorizationRequired
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
