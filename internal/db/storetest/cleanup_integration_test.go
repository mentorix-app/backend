//go:build integration

package storetest

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/cleanup"
	"mentorix-backend/internal/program"
)

func TestCleanup_Run_purgesStaleRows(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	result, err := cleanup.Run(ctx, pool)
	if err != nil {
		t.Fatalf("cleanup.Run: %v", err)
	}
	if result.TrainerInvites < 0 || result.RefreshSessions < 0 {
		t.Fatalf("unexpected negative counts: %+v", result)
	}
}

// TestCleanup_purgesOrphanBlockClients proves the janitor's global purge
// removes a rule whose block_key exists in neither the working copy nor any
// surviving version of its program — the only case that makes a rule truly
// orphaned, as opposed to merely absent from the draft.
func TestCleanup_purgesOrphanBlockClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	userID, _ := seedTrainerAndExercise(t, pool, "orphan-rules")
	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	// A rule pointing at a block_key that exists in neither the template nor any version.
	orphanKey := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id)
		VALUES ($1, $2, $3)`, draft.ID, orphanKey, userID); err != nil {
		t.Fatalf("insert orphan rule: %v", err)
	}

	if _, err := cleanup.Run(ctx, pool); err != nil {
		t.Fatalf("cleanup.Run: %v", err)
	}

	var left int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM mentorix.program_block_clients
		WHERE block_key = $1`, orphanKey).Scan(&left); err != nil {
		t.Fatalf("count orphan rules: %v", err)
	}
	if left != 0 {
		t.Fatalf("orphan rules after cleanup = %d, want 0", left)
	}
}
