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

// DeleteClientAccount deletes a client or a trainer without business data, and every row
// that belongs to them, in one transaction. It removes the invites the account consumed and,
// with the user row, the trainers row with the trainer's own invites and admin-granted
// entitlements. What the trainer deleted in the app is not business data: the soft-deleted
// programs the account created, with their trees, versions and block visibility rules, and
// then the soft-deleted exercises its trainers row owns, are removed before the user row (see
// TrainerHasBusinessData for which of them qualify). It returns the Telegram user id of the
// account, empty when there is none.
//
// revokedAppleSubject is the subject of the Apple sign-in whose authorization was
// revoked at Apple just before, empty when none was. An account that has Apple
// sign-ins is deleted only when that subject is one of them, so one revoked sign-in
// is enough for an account with two (a known limit).
//
// Locks are taken in the order the other writers use: the trainer's invite rows (accepting
// an invite locks its row first), then the trainers row (quota writers lock it before they
// touch the user row), then the user row. An assignment committed at the same moment is
// therefore either seen and deleted too or fails on a user that is gone, and an invite
// accepted at the same moment normally commits first, so the new client makes the deletion
// refuse, or finds the invite gone. Every refusal is decided under those locks, and nothing
// is changed when one applies, in this order: ErrAccountDeletionUnavailable when the roles are
// not exactly client, exactly trainer, or both, then ErrAccountDeletionUnavailable when the
// account holds business data (see TrainerHasBusinessData, checked for every account), then
// ErrAppleSignInNotRevoked when the account has an Apple sign-in and revokedAppleSubject is
// not one of them. ErrAccountDeletionUnavailable is also returned when the database refuses
// a delete because other rows still reference the user or a purged program or exercise.
// pgx.ErrNoRows means the user does not exist.
//
// The purge takes no explicit program lock. The deletion holds the trainers row, then the
// user row, so it reaches programs after the trainers row, the same relative order as
// SetClientProgramAssignment (trainers row, then program row), and its DELETE locks the
// program rows it removes anyway. A new assignment cannot slip in unseen: only the creator of
// a program can assign it, a soft-deleted program is refused, and assigning locks the
// trainers row, which the deletion holds FOR UPDATE. Completions: every completion carries a
// trainer_id (a NO ACTION reference to trainers), so inserting one waits for the deletion's
// trainers-row lock; in practice a completion on a program of this account carries this
// trainers row and makes the check refuse.
//
// Known windows, each ending in a database error for one of the two sides, with nothing
// changed and a repeat that works:
//   - "become a trainer" commits after the trainers row was looked up (there was none) and
//     before the user row is locked: the deletion holds the user lock without the new trainers
//     row, and a quota writer on that row can deadlock with it.
//   - The invite lock is a READ COMMITTED snapshot, so an invite created after it and accepted
//     before the delete cascade reaches the invite rows can deadlock with the deletion.
//   - The janitor's PurgeStaleTrainerInvites deletes invite rows in scan order and can
//     deadlock with the deletion the same way the old cascade could.
//   - A program edit in flight (ensureMutable checks before its transaction and does not
//     re-check deleted_at under the program lock) while the same trainer soft-deletes that
//     program and deletes the account can deadlock (40P01) with the purge: the edit locks the
//     program row and then, when it sets modified_by, takes KEY SHARE on the user row through
//     that foreign key, while the purge takes the user row first and then the program rows.
func (s *Store) DeleteClientAccount(ctx context.Context, userID uuid.UUID, revokedAppleSubject string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	userPG := pgconv.ToPGUUID(userID)

	if _, err := qtx.LockTrainerInvitesByUserID(ctx, userPG); err != nil {
		return "", fmt.Errorf("lock trainer invites: %w", err)
	}
	if _, err := qtx.LockTrainerByUserID(ctx, userPG); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lock trainer: %w", err)
	}
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
	if !deletableRoles(roles) {
		return "", ErrAccountDeletionUnavailable
	}
	hasData, err := qtx.TrainerHasBusinessData(ctx, userPG)
	if err != nil {
		return "", fmt.Errorf("check trainer data: %w", err)
	}
	if hasData {
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

	// Programs go before exercises: a program tree that uses an exercise holds a RESTRICT
	// reference to it. Under the locks above every row these two delete passed the check.
	// As a safety net, an assignment that still references a purged program makes the database
	// refuse (ErrAccountDeletionUnavailable, nothing changed). A workout completion the app
	// writes carries the account's trainers row, so it cannot appear before the deletion
	// commits; its insert waits for the deletion's lock.
	if _, err := qtx.PurgeDeletedProgramsByCreator(ctx, userPG); err != nil {
		return "", deletionError("purge deleted programs", err)
	}
	if _, err := qtx.PurgeDeletedExercisesByOwner(ctx, userPG); err != nil {
		return "", deletionError("purge deleted exercises", err)
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
// The role rule and the trainer data rule also run here, ahead of the store, so that Apple is
// not asked to revoke for an account that is already refused. This check takes no lock, so it
// is not a guarantee: a role or trainer data that arrives between it and the store's locks
// (a client accepting an invite while the Apple call runs, say) is seen only by the store.
// Apple has then revoked, the store answers ErrAccountDeletionUnavailable, the account stays,
// and the person signs in with Apple again. The other refusals stay in the store.
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
	if !deletableRoles(roles) {
		return "", ErrAccountDeletionUnavailable
	}
	hasData, err := s.store.TrainerHasBusinessData(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("check trainer data: %w", err)
	}
	if hasData {
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

// TrainerHasBusinessData reports whether the account holds data that deleting it must not
// destroy, without taking any lock: a client of the trainer (any status, blocked included), a
// program or exercise they authored or last edited, a program week, day or block exercise
// they last edited, a program version they published, a program assignment made or last
// edited by them or made for the trainer, a workout completion or a comment on one, a block
// visibility rule they created, or an entitlement paid through a store (any status). Invites
// and admin-granted entitlements are not data: they are removed with the account. A user
// without a trainers row has only what they authored, edited or published.
//
// What the trainer deleted is not data either. A soft-deleted program the account created
// does not count, nor the rows the account holds inside its trees, when no assignment and no
// workout completion references it or one of its versions. A soft-deleted exercise owned by
// the account's trainers row does not count when no other program or version tree uses it.
// The rest of the soft-deleted rows still count: an exercise the account created or last
// edited that the account's trainers row does not own (a global one included), and a program
// that another trainer created and the account last edited.
func (s *Store) TrainerHasBusinessData(ctx context.Context, userID uuid.UUID) (bool, error) {
	has, err := s.q.TrainerHasBusinessData(ctx, pgconv.ToPGUUID(userID))
	if err != nil {
		return false, fmt.Errorf("check trainer data: %w", err)
	}
	return has, nil
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

// deletableRoles reports whether the app may delete an account with these roles: exactly
// client, trainer, or both. An admin, alone or with other roles, a repeated role and an
// account with no role row are refused.
func deletableRoles(roles []string) bool {
	var hasClient, hasTrainer bool
	for _, role := range roles {
		switch {
		case role == RoleClient && !hasClient:
			hasClient = true
		case role == RoleTrainer && !hasTrainer:
			hasTrainer = true
		default:
			return false
		}
	}
	return hasClient || hasTrainer
}

// DeleteAccount deletes the account of a client, or of a trainer without business data, and
// everything that belongs to it.
//
// An account with an Apple sign-in is first revoked at Apple. Nothing is sent to Apple
// until six checks have passed, in this order: the roles are client, trainer or both
// (ErrAccountDeletionUnavailable), the trainer side holds no business data
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
// The checks before Apple take no lock, so they are not a guarantee: a role or trainer data
// that arrives while the Apple call runs makes the store refuse after Apple has revoked.
// The account stays and the person signs in with Apple again, the same window as a database
// failure after a revocation.
//
// The store decides who may be deleted, under its locks. An Apple sign-in that arrived
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
