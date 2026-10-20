//go:build integration

package storetest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

// promoteToAdminOnly converts a trainer-only account into a pure admin
// (admin role is exclusive with trainer/client).
func promoteToAdminOnly(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `DELETE FROM mentorix.user_roles WHERE user_id = $1 AND role = 'trainer'`, userID)
	if err != nil {
		t.Fatalf("revoke trainer role: %v", err)
	}
	_, err = pool.Exec(ctx, `DELETE FROM mentorix.trainers WHERE user_id = $1`, userID)
	if err != nil {
		t.Fatalf("delete trainer profile: %v", err)
	}
	if err := auth.NewStore(pool).GrantRole(ctx, userID, auth.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
}

// newTrainer creates a trainer with a Google sign-in identity (subject = email) and the
// trainer role, the way production creates one apart from the Become a trainer
// terms acceptance: the trainers row has NULL terms, like an account that predates them.
// Call it from the test goroutine only: it reports failures with t.Fatalf.
func newTrainer(t *testing.T, pool *pgxpool.Pool, email, displayName string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin trainer tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO mentorix.users (primary_email, display_name) VALUES ($1, $2) RETURNING id`,
		email, displayName).Scan(&userID); err != nil {
		t.Fatalf("insert trainer user %q: %v", email, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO mentorix.auth_identities (user_id, provider, subject) VALUES ($1, $2, $3)`,
		userID, auth.ProviderGoogle, email); err != nil {
		t.Fatalf("insert trainer identity %q: %v", email, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO mentorix.user_roles (user_id, role) VALUES ($1, $2)`,
		userID, auth.RoleTrainer); err != nil {
		t.Fatalf("insert trainer role %q: %v", email, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO mentorix.trainers (user_id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert trainers row %q: %v", email, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit trainer %q: %v", email, err)
	}
	return userID
}
