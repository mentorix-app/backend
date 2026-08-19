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

// TestCleanup_preservesRuleStillLiveInAssignedVersion covers the direction
// this task exists to protect, not just the harmless one: a block deleted
// from the working copy (trainer removed it or merged it away) whose
// block_key still lives inside a frozen version a client is assigned to
// right now. Its visibility rule must survive both purge paths — deleting it
// would silently un-restrict that block for every client on that version. In
// the same run it also proves neither purge is a no-op by re-checking a
// fully orphaned key (the harmless direction TestCleanup_purgesOrphanBlockClients
// already covers) is gone after each call.
func TestCleanup_preservesRuleStillLiveInAssignedVersion(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	// n=2: SetBlockClients refuses to restrict a day's last shared block, so
	// blocks[0] must stay shared while blocks[1] is restricted below.
	programID, weekID, blocks, clientUserID := seedDayWithBlocks(t, pool, "surviving-rule", 2)
	trainerUserID := ownerUserID(t, pool, programID)
	survivingKey := blocks[1].BlockKey

	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, weekID, blocks[1].ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}

	// Delete blocks[1] from the working copy by deleting its only exercise —
	// single blocks disappear once empty (see DeleteBlockExercise). Its
	// block_key now lives only inside the frozen version the client was
	// assigned to at publish time (seedDayWithBlocks publishes exactly once).
	itemID := blocks[1].Exercises[0].ID
	if _, err := store.DeleteBlockExercise(ctx, programID, weekID, blocks[1].ID, itemID); err != nil {
		t.Fatalf("DeleteBlockExercise: %v", err)
	}

	// Confirm the test's premise before trusting its assertions: the working
	// copy really no longer has this block_key.
	var stillInWorkingCopy bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM mentorix.program_week_day_blocks b
			JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id
			WHERE d.program_id = $1 AND b.block_key = $2)`,
		programID, survivingKey).Scan(&stillInWorkingCopy); err != nil {
		t.Fatalf("check working copy for block_key: %v", err)
	}
	if stillInWorkingCopy {
		t.Fatal("test premise broken: block_key still present in the working copy after DeleteBlockExercise")
	}

	countRule := func(key uuid.UUID) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM mentorix.program_block_clients
			WHERE program_id = $1 AND block_key = $2 AND client_user_id = $3`,
			programID, key, clientUserID).Scan(&n); err != nil {
			t.Fatalf("count rule for %s: %v", key, err)
		}
		return n
	}
	if got := countRule(survivingKey); got != 1 {
		t.Fatalf("surviving rule before any purge = %d, want 1", got)
	}

	insertOrphan := func() uuid.UUID {
		t.Helper()
		orphanKey := uuid.New()
		if _, err := pool.Exec(ctx, `
			INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id)
			VALUES ($1, $2, $3)`, programID, orphanKey, clientUserID); err != nil {
			t.Fatalf("insert orphan rule: %v", err)
		}
		return orphanKey
	}

	// Scoped purge, via the store's own CleanupProgramVersions (this program
	// has exactly one version, so the version-cleanup loop itself deletes
	// nothing — the purge call still has to run and still has to leave the
	// surviving rule alone).
	orphanKey := insertOrphan()
	if _, err := store.CleanupProgramVersions(ctx, programID); err != nil {
		t.Fatalf("CleanupProgramVersions: %v", err)
	}
	if got := countRule(survivingKey); got != 1 {
		t.Fatalf("surviving rule after CleanupProgramVersions = %d, want 1 (still assigned to a live version)", got)
	}
	if got := countRule(orphanKey); got != 0 {
		t.Fatalf("orphan rule after CleanupProgramVersions = %d, want 0 (purge must not be a no-op)", got)
	}

	// Global purge, via the janitor's entry point — reinsert the orphan since
	// the scoped call above already consumed it, so this call is proven
	// non-vacuous too, not just relying on the previous call's work.
	orphanKey = insertOrphan()
	if _, err := cleanup.Run(ctx, pool); err != nil {
		t.Fatalf("cleanup.Run: %v", err)
	}
	if got := countRule(survivingKey); got != 1 {
		t.Fatalf("surviving rule after cleanup.Run = %d, want 1 (still assigned to a live version)", got)
	}
	if got := countRule(orphanKey); got != 0 {
		t.Fatalf("orphan rule after cleanup.Run = %d, want 0 (purge must not be a no-op)", got)
	}
}
