//go:build integration

package storetest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
)

// promoteToAdminOnly converts a registered trainer into a pure admin
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
