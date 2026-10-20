//go:build integration

package storetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

// raceContext bounds the test so that a stuck goroutine cannot hang the run. Call it after
// NewPool: cleanups run last in, first out, so the cancel runs before the pool closes.
func raceContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// newMergePair creates a Telegram owner and an app account whose ids sort in the wanted order,
// the order in which LinkTelegram locks the two rows. The app account is an empty Google client,
// or a trainer-only account when appIsTrainer is set. Each attempt makes a fresh pair, so a miss
// has a probability of one half. It returns the Telegram subject of the owner.
func newMergePair(t *testing.T, pool *pgxpool.Pool, store *auth.Store, appBelowOwner, appIsTrainer bool) (owner, app uuid.UUID, telegramSubject string) {
	t.Helper()
	for i := range 64 {
		telegramSubject = fmt.Sprintf("tg-race-%d", i)
		owner = newClientWithIdentity(t, store, auth.ProviderTelegram, telegramSubject, "")
		if appIsTrainer {
			app = newTrainer(t, pool, fmt.Sprintf("app-trainer-%d@test.com", i), "Coach")
		} else {
			app = newClientWithIdentity(t, store, auth.ProviderGoogle, fmt.Sprintf("g-app-%d", i), "app@test.com")
		}
		if (bytes.Compare(app[:], owner[:]) < 0) == appBelowOwner {
			return owner, app, telegramSubject
		}
	}
	t.Fatalf("no pair with the app id %s the owner id after 64 tries", map[bool]string{true: "below", false: "above"}[appBelowOwner])
	return uuid.Nil, uuid.Nil, ""
}

// waitForWaitersOrResult returns once want sessions wait for a lock, with finished false, or
// once done delivered a result, with finished true and that result.
func waitForWaitersOrResult(t *testing.T, pool *pgxpool.Pool, want int, done <-chan error) (finished bool, err error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			return true, err
		default:
		}
		if lockWaiters(t, pool) >= want {
			return false, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no result and fewer than %d sessions waiting for a lock after 5 seconds", want)
	return false, nil
}

// receiveWithin waits for the result of a goroutine that has not delivered it yet.
func receiveWithin(t *testing.T, name string, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not finish within 10 seconds", name)
		return nil
	}
}

// lockOwnerRow starts a transaction that holds the user row FOR UPDATE until it ends.
func lockOwnerRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner uuid.UUID) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(ctx, `SELECT id FROM mentorix.users WHERE id = $1 FOR UPDATE`, owner); err != nil {
		t.Fatalf("lock owner row: %v", err)
	}
	return tx
}

// Pins the race between the merge and a trainer activation of the app account. The merge
// checks that the app account is empty and deletes it later; an activation committed in
// between used to be removed with the user row by the cascade while it had answered success.
// The merge now locks both users first, so the activation either waits and finds the user
// gone, or commits first and the merge sees the trainer role and refuses.
func TestAuthStore_LinkTelegram_trainerActivationDuringMergeIsNotLost(t *testing.T) {
	for _, tc := range []struct {
		name          string
		appBelowOwner bool
	}{
		{"app id lower", true},
		{"app id higher", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := NewPool(t)
			ctx := raceContext(t)
			store := auth.NewStore(pool)
			owner, app, telegramSubject := newMergePair(t, pool, store, tc.appBelowOwner, false)
			tx := lockOwnerRow(t, ctx, pool, owner)

			linkDone := make(chan error, 1)
			go func() {
				_, err := store.LinkTelegram(ctx, app, telegramSubject)
				linkDone <- err
			}()
			waitForLockWait(t, pool)

			activateDone := make(chan error, 1)
			go func() { activateDone <- store.ActivateTrainer(ctx, app, "2026-10-01") }()
			activateFinished, activateErr := waitForWaitersOrResult(t, pool, 2, activateDone)

			if err := tx.Commit(ctx); err != nil {
				t.Fatalf("release owner row: %v", err)
			}
			linkErr := receiveWithin(t, "LinkTelegram", linkDone)
			if !activateFinished {
				activateErr = receiveWithin(t, "ActivateTrainer", activateDone)
			}

			if tc.appBelowOwner {
				if linkErr != nil {
					t.Fatalf("LinkTelegram() error = %v", linkErr)
				}
				if !errors.Is(activateErr, pgx.ErrNoRows) {
					t.Errorf("ActivateTrainer() error = %v, want pgx.ErrNoRows", activateErr)
				}
				if userExists(t, pool, app) {
					t.Error("app user still exists")
				}
				if got := trainerTermsOf(t, pool, app); got.found {
					t.Errorf("a trainers row exists for the removed user: %+v", got)
				}
				if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE user_id = $1 AND provider = $2`, owner, auth.ProviderGoogle); n != 1 {
					t.Errorf("Google identities of the owner = %d, want 1", n)
				}
				return
			}
			if activateErr != nil {
				t.Fatalf("ActivateTrainer() error = %v", activateErr)
			}
			if !errors.Is(linkErr, auth.ErrTelegramLinkConflict) {
				t.Errorf("LinkTelegram() error = %v, want ErrTelegramLinkConflict", linkErr)
			}
			if !userExists(t, pool, app) || !userExists(t, pool, owner) {
				t.Fatal("a refused merge removed an account")
			}
			assertTerms(t, trainerTermsOf(t, pool, app), "2026-10-01")
			assertRoles(t, store, app, auth.RoleClient, auth.RoleTrainer)
			if got, ok := identityOwner(t, pool, auth.ProviderTelegram, telegramSubject); !ok || got != owner {
				t.Errorf("telegram owner = %v, %v, want %v", got, ok, owner)
			}
			if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE user_id = $1 AND provider = $2`, app, auth.ProviderGoogle); n != 1 {
				t.Errorf("Google identities of the app account = %d, want 1", n)
			}
			if got := trainerTermsOf(t, pool, owner); got.found {
				t.Errorf("the owner got a trainers row: %+v", got)
			}
		})
	}
}

// Pins the second race: the Telegram account is deleted while the merge waits for it. The
// foreign key violation of the identity move used to come back as a generic error (HTTP 500);
// the merge now locks the owner row, finds it gone and answers a conflict.
func TestAuthStore_LinkTelegram_telegramAccountDeletedDuringMergeIsConflict(t *testing.T) {
	for _, tc := range []struct {
		name          string
		appBelowOwner bool
	}{
		{"app id lower", true},
		{"app id higher", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := NewPool(t)
			ctx := raceContext(t)
			store := auth.NewStore(pool)
			owner, app, telegramSubject := newMergePair(t, pool, store, tc.appBelowOwner, false)
			tx := lockOwnerRow(t, ctx, pool, owner)

			linkDone := make(chan error, 1)
			go func() {
				_, err := store.LinkTelegram(ctx, app, telegramSubject)
				linkDone <- err
			}()
			waitForLockWait(t, pool)
			if _, err := tx.Exec(ctx, `DELETE FROM mentorix.users WHERE id = $1`, owner); err != nil {
				t.Fatalf("delete owner: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatalf("commit: %v", err)
			}

			linkErr := receiveWithin(t, "LinkTelegram", linkDone)

			if !errors.Is(linkErr, auth.ErrTelegramLinkConflict) {
				t.Errorf("LinkTelegram() error = %v, want ErrTelegramLinkConflict", linkErr)
			}
			if !userExists(t, pool, app) {
				t.Fatal("the app user was removed")
			}
			if n := countRows(t, pool, `SELECT count(*) FROM mentorix.auth_identities WHERE user_id = $1 AND provider = $2`, app, auth.ProviderGoogle); n != 1 {
				t.Errorf("Google identities of the app account = %d, want 1", n)
			}
		})
	}
}

// Pins a lock cycle between the merge and the writers of a non-empty app account. A trainer
// write holds a key-share lock on the app user row and next needs one on the owner row; a merge
// that already holds the owner row FOR UPDATE and waits for the app row closes the cycle, and
// PostgreSQL aborts one side with 40P01. The merge would refuse that account anyway, so it
// now answers the conflict before it takes any lock.
func TestAuthStore_LinkTelegram_nonEmptyAppAccountDoesNotDeadlockWithItsWriters(t *testing.T) {
	pool := NewPool(t)
	ctx := raceContext(t)
	store := auth.NewStore(pool)
	// The owner id is lower, so the merge would take the owner row first.
	owner, app, telegramSubject := newMergePair(t, pool, store, false, true)

	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = writer.Rollback(context.Background()) })
	const insertSession = `
INSERT INTO mentorix.auth_refresh_sessions (user_id, token_hash, expires_at)
VALUES ($1, $2, now() + interval '1 hour')`
	if _, err := writer.Exec(ctx, insertSession, app, []byte("hash-of-app")); err != nil {
		t.Fatalf("writer touches the app user: %v", err)
	}

	linkDone := make(chan error, 1)
	go func() {
		_, err := store.LinkTelegram(ctx, app, telegramSubject)
		linkDone <- err
	}()
	linkFinished, linkErr := waitForWaitersOrResult(t, pool, 1, linkDone)

	_, writeErr := writer.Exec(ctx, insertSession, owner, []byte("hash-of-owner"))
	if writeErr == nil {
		writeErr = writer.Commit(ctx)
	} else {
		_ = writer.Rollback(ctx)
	}
	if !linkFinished {
		linkErr = receiveWithin(t, "LinkTelegram", linkDone)
	}

	var pgErr *pgconn.PgError
	if errors.As(writeErr, &pgErr) || errors.As(linkErr, &pgErr) {
		t.Errorf("a database error reached the caller: %v / %v", writeErr, linkErr)
	}
	if writeErr != nil {
		t.Errorf("the writer failed: %v", writeErr)
	}
	if !errors.Is(linkErr, auth.ErrTelegramLinkConflict) {
		t.Errorf("LinkTelegram() error = %v, want ErrTelegramLinkConflict", linkErr)
	}
	if !userExists(t, pool, app) || !userExists(t, pool, owner) {
		t.Error("a refused merge removed an account")
	}
}
