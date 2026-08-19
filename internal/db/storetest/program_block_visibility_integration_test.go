//go:build integration

package storetest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
)

func TestMigration_BlockKeyColumnsExist(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	// Both block_key columns must be NOT NULL *and* carry a generated default:
	// the existing insert queries do not mention block_key until Tasks 2 and 3,
	// so without the default this migration breaks every block insert.
	for _, table := range []string{"program_week_day_blocks", "program_version_week_day_blocks"} {
		var isNullable string
		var columnDefault *string
		err := pool.QueryRow(ctx, `
			SELECT is_nullable, column_default
			FROM information_schema.columns
			WHERE table_schema = 'mentorix' AND table_name = $1 AND column_name = 'block_key'`,
			table).Scan(&isNullable, &columnDefault)
		if err != nil {
			t.Fatalf("query %s.block_key: %v", table, err)
		}
		if isNullable != "NO" {
			t.Fatalf("%s.block_key is_nullable = %q, want NO", table, isNullable)
		}
		if columnDefault == nil || !strings.Contains(*columnDefault, "gen_random_uuid") {
			t.Fatalf("%s.block_key default = %v, want gen_random_uuid()", table, columnDefault)
		}
	}

	var hasTable bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'mentorix' AND table_name = 'program_block_clients')`).Scan(&hasTable)
	if err != nil {
		t.Fatalf("query information_schema: %v", err)
	}
	if !hasTable {
		t.Fatal("table mentorix.program_block_clients does not exist")
	}

	var hasUniq bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint
			WHERE conname = 'program_block_clients_program_block_key_client_uniq')`).Scan(&hasUniq)
	if err != nil {
		t.Fatalf("query pg_constraint: %v", err)
	}
	if !hasUniq {
		t.Fatal("unique constraint on program_block_clients is missing")
	}
}

// seedTrainerAndExercise registers a trainer and one catalog exercise.
// Returns the trainer's user id and the exercise id.
func seedTrainerAndExercise(t *testing.T, pool *pgxpool.Pool, emailPrefix string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := auth.NewStore(pool).RegisterTrainerEmailPassword(
		ctx, emailPrefix+"@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	ex, err := exercise.NewStore(pool).Create(ctx, userID, nil, exercise.UpsertInput{
		Name:        "Bench Press",
		NameRu:      "Жим",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}
	return userID, ex.ID
}

func TestProgramDetail_BlockCarriesKeyAndEmptyClients(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "block-key-detail")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]

	detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}

	block, ok := firstDayBlock(detail.Weeks[0].Days[0])
	if !ok {
		t.Fatal("expected one block in day")
	}
	if block.BlockKey == uuid.Nil {
		t.Fatal("block.BlockKey is uuid.Nil, want generated key")
	}
	if len(block.ClientUserIDs) != 0 {
		t.Fatalf("block.ClientUserIDs = %v, want empty (shared block)", block.ClientUserIDs)
	}
}

// TestProgramDetail_BlockClientRules_ScopedByBlockKey covers the "block has
// rules" path end to end through the real ListProgramBlockClients query: it
// inserts a row directly into mentorix.program_block_clients (the store has
// no writer for this table until Task 5) and checks that only the block
// whose block_key matches picks up the rule, while a sibling block in the
// same day stays empty. TestProgramDetail_BlockCarriesKeyAndEmptyClients only
// covers the zero-rows case, which would still pass even if the query scanned
// block_key/client_user_id into the wrong columns.
func TestProgramDetail_BlockClientRules_ScopedByBlockKey(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	trainerID, exerciseID := seedTrainerAndExercise(t, pool, "block-rules-trainer")

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	clientID, err := auth.NewStore(pool).RegisterTrainerEmailPassword(
		ctx, "block-rules-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	detail, err := store.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]

	// Restricted block: gets a visibility rule below.
	detail, err = createSingleBlockStore(ctx, store, trainerID, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore (restricted block): %v", err)
	}
	// Shared block: no rule row, must stay visible to everyone.
	detail, err = createSingleBlockStore(ctx, store, trainerID, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore (shared block): %v", err)
	}

	day = detail.Weeks[0].Days[0]
	if len(day.Blocks) != 2 {
		t.Fatalf("expected 2 blocks in day, got %d", len(day.Blocks))
	}
	restrictedBlock, sharedBlock := day.Blocks[0], day.Blocks[1]

	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id)
		VALUES ($1, $2, $3)`,
		detail.ID, restrictedBlock.BlockKey, clientID); err != nil {
		t.Fatalf("insert program_block_clients row: %v", err)
	}

	detail, err = store.GetDetail(ctx, detail.ID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	day = detail.Weeks[0].Days[0]
	if len(day.Blocks) != 2 {
		t.Fatalf("expected 2 blocks in day after reload, got %d", len(day.Blocks))
	}

	var reloadedRestricted, reloadedShared program.DayBlock
	var foundRestricted, foundShared bool
	for _, b := range day.Blocks {
		switch b.ID {
		case restrictedBlock.ID:
			reloadedRestricted, foundRestricted = b, true
		case sharedBlock.ID:
			reloadedShared, foundShared = b, true
		}
	}
	if !foundRestricted || !foundShared {
		t.Fatalf("could not find both blocks after reload: restricted=%v shared=%v", foundRestricted, foundShared)
	}

	if len(reloadedRestricted.ClientUserIDs) != 1 || reloadedRestricted.ClientUserIDs[0] != clientID {
		t.Fatalf("restricted block ClientUserIDs = %v, want [%v]", reloadedRestricted.ClientUserIDs, clientID)
	}
	if len(reloadedShared.ClientUserIDs) != 0 {
		t.Fatalf("shared block ClientUserIDs = %v, want empty (no rule row)", reloadedShared.ClientUserIDs)
	}
}

func TestBlockKey_SurvivesPublishAndDiscard(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "block-key-publish")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]
	detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}
	block, _ := firstDayBlock(detail.Weeks[0].Days[0])
	wantKey := block.BlockKey

	// Publish freezes the tree; the frozen block must keep the same key.
	published, err := store.PublishFromDraft(ctx, detail.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	_ = published

	var frozenKey uuid.UUID
	err = pool.QueryRow(ctx, `
		SELECT vb.block_key
		FROM mentorix.program_version_week_day_blocks vb
		JOIN mentorix.program_version_week_days vd ON vd.id = vb.program_version_week_day_id
		JOIN mentorix.program_versions v ON v.id = vd.program_version_id
		WHERE v.program_id = $1`, detail.ID).Scan(&frozenKey)
	if err != nil {
		t.Fatalf("query frozen block_key: %v", err)
	}
	if frozenKey != wantKey {
		t.Fatalf("frozen block_key = %s, want %s", frozenKey, wantKey)
	}

	// Discard rebuilds the working copy from the latest version; the key must survive.
	restored, err := store.RestoreWorkingTreeFromLatestVersion(ctx, detail.ID, userID)
	if err != nil {
		t.Fatalf("RestoreWorkingTreeFromLatestVersion: %v", err)
	}
	restoredBlock, ok := firstDayBlock(restored.Weeks[0].Days[0])
	if !ok {
		t.Fatal("expected one block after restore")
	}
	if restoredBlock.BlockKey != wantKey {
		t.Fatalf("restored block_key = %s, want %s", restoredBlock.BlockKey, wantKey)
	}
}

// seedAssignedClient registers a client user and links them to the trainer.
// Returns the client's user id and the trainer id.
func seedAssignedClient(t *testing.T, pool *pgxpool.Pool, trainerUserID uuid.UUID, emailPrefix string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	clientUserID, err := auth.NewStore(pool).RegisterTrainerEmailPassword(
		ctx, emailPrefix+"@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client user: %v", err)
	}

	var trainerID uuid.UUID
	err = pool.QueryRow(ctx,
		`SELECT id FROM mentorix.trainers WHERE user_id = $1`, trainerUserID).Scan(&trainerID)
	if err != nil {
		t.Fatalf("select trainer id: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')`, trainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}
	return clientUserID, trainerID
}

func TestSetBlockClients_refusesLastSharedBlock(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "last-shared")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]

	for i := 0; i < 2; i++ {
		detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
			program.DayExerciseInput{ExerciseID: exerciseID})
		if err != nil {
			t.Fatalf("createSingleBlockStore %d: %v", i, err)
		}
	}
	blocks := detail.Weeks[0].Days[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}

	published, err := store.PublishFromDraft(ctx, detail.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	clientUserID, trainerID := seedAssignedClient(t, pool, userID, "last-shared-client")
	programID := published.ID
	if _, err := store.SetClientProgramAssignment(ctx, userID, trainerID, clientUserID, &programID); err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	// Restricting the first block is fine — the second one stays shared.
	if _, err := store.SetBlockClients(ctx, userID, programID, week.ID, blocks[0].ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients on first block: %v", err)
	}

	// Restricting the second one would leave the day without a shared block.
	_, err = store.SetBlockClients(ctx, userID, programID, week.ID, blocks[1].ID,
		[]uuid.UUID{clientUserID})
	if !errors.Is(err, program.ErrLastSharedBlock) {
		t.Fatalf("SetBlockClients on last shared block error = %v, want ErrLastSharedBlock", err)
	}
}

func TestSetBlockClients_refusesUnassignedClient(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "unassigned")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]
	detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}
	block, _ := firstDayBlock(detail.Weeks[0].Days[0])

	stranger, _ := seedAssignedClient(t, pool, userID, "unassigned-client")
	_, err = store.SetBlockClients(ctx, userID, detail.ID, week.ID, block.ID,
		[]uuid.UUID{stranger})
	if !errors.Is(err, program.ErrClientNotAssignedToProgram) {
		t.Fatalf("SetBlockClients error = %v, want ErrClientNotAssignedToProgram", err)
	}
}

// blockByID finds a block by id anywhere in the detail's first day. Test
// helper only: the fixtures below always operate on a single week/day.
func blockByID(d program.Detail, id uuid.UUID) (program.DayBlock, bool) {
	for _, b := range d.Weeks[0].Days[0].Blocks {
		if b.ID == id {
			return b, true
		}
	}
	return program.DayBlock{}, false
}

// TestSetBlockClients_concurrentRestrictKeepsSharedBlock is a regression test
// for the race where two requests restrict different blocks of the same day
// at once: without a serializing lock, each reads a snapshot where the
// other's block is still shared, both pass the day-invariant check, and both
// commit — leaving the day with zero shared blocks. With the row lock in
// place, exactly one of the two must lose the race and be refused.
func TestSetBlockClients_concurrentRestrictKeepsSharedBlock(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "race")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]

	for i := 0; i < 2; i++ {
		detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
			program.DayExerciseInput{ExerciseID: exerciseID})
		if err != nil {
			t.Fatalf("createSingleBlockStore %d: %v", i, err)
		}
	}
	blocks := detail.Weeks[0].Days[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	blockAID, blockBID := blocks[0].ID, blocks[1].ID

	published, err := store.PublishFromDraft(ctx, detail.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	clientUserID, trainerID := seedAssignedClient(t, pool, userID, "race-client")
	programID := published.ID
	if _, err := store.SetClientProgramAssignment(ctx, userID, trainerID, clientUserID, &programID); err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	// A single race attempt rarely lands the two goroutines' reads in the same
	// window on a fast local database, so this repeats the experiment, resetting
	// both blocks to shared between attempts. Without the row lock this fails
	// reliably within a handful of attempts (observed 1-in-8 to 1-in-10 locally);
	// with it, every attempt must land exactly one ErrLastSharedBlock.
	const attempts = 15
	for attempt := 0; attempt < attempts; attempt++ {
		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, errs[0] = store.SetBlockClients(ctx, userID, programID, week.ID, blockAID,
				[]uuid.UUID{clientUserID})
		}()
		go func() {
			defer wg.Done()
			<-start
			_, errs[1] = store.SetBlockClients(ctx, userID, programID, week.ID, blockBID,
				[]uuid.UUID{clientUserID})
		}()
		close(start)
		wg.Wait()

		lastSharedCount := 0
		for _, err := range errs {
			switch {
			case errors.Is(err, program.ErrLastSharedBlock):
				lastSharedCount++
			case err != nil:
				t.Fatalf("attempt %d: unexpected SetBlockClients error: %v", attempt, err)
			}
		}
		if lastSharedCount != 1 {
			t.Fatalf("attempt %d: ErrLastSharedBlock count = %d, want exactly 1 (errs=%v)",
				attempt, lastSharedCount, errs)
		}

		final, err := store.GetDetail(ctx, programID)
		if err != nil {
			t.Fatalf("attempt %d: GetDetail: %v", attempt, err)
		}
		sharedCount := 0
		for _, b := range final.Weeks[0].Days[0].Blocks {
			if len(b.ClientUserIDs) == 0 {
				sharedCount++
			}
		}
		if sharedCount == 0 {
			t.Fatalf("attempt %d: day has no shared block after concurrent restrict: %+v",
				attempt, final.Weeks[0].Days[0].Blocks)
		}

		// Reset both blocks back to shared before the next attempt. Clearing a
		// restriction can never break the invariant, so both must succeed.
		if _, err := store.SetBlockClients(ctx, userID, programID, week.ID, blockAID, nil); err != nil {
			t.Fatalf("attempt %d: reset block A: %v", attempt, err)
		}
		if _, err := store.SetBlockClients(ctx, userID, programID, week.ID, blockBID, nil); err != nil {
			t.Fatalf("attempt %d: reset block B: %v", attempt, err)
		}
	}
}

// TestSetBlockClients_fullReplace covers the headline PUT semantics: a
// second call with a different (or empty) list must fully replace the
// previous rule, not append to it. ON CONFLICT DO NOTHING on the insert
// would make this endpoint silently append-only, so this asserts the
// resulting client_user_ids after each call rather than just the error.
func TestSetBlockClients_fullReplace(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "full-replace")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]

	// Two blocks: the second one keeps the day's invariant satisfied while
	// the first one is restricted below.
	for i := 0; i < 2; i++ {
		detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
			program.DayExerciseInput{ExerciseID: exerciseID})
		if err != nil {
			t.Fatalf("createSingleBlockStore %d: %v", i, err)
		}
	}
	target := detail.Weeks[0].Days[0].Blocks[0].ID

	published, err := store.PublishFromDraft(ctx, detail.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	clientUserID, trainerID := seedAssignedClient(t, pool, userID, "full-replace-client")
	programID := published.ID
	if _, err := store.SetClientProgramAssignment(ctx, userID, trainerID, clientUserID, &programID); err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	restricted, err := store.SetBlockClients(ctx, userID, programID, week.ID, target,
		[]uuid.UUID{clientUserID})
	if err != nil {
		t.Fatalf("SetBlockClients (restrict): %v", err)
	}
	gotRestricted, ok := blockByID(restricted, target)
	if !ok {
		t.Fatal("target block not found after restrict")
	}
	if len(gotRestricted.ClientUserIDs) != 1 || gotRestricted.ClientUserIDs[0] != clientUserID {
		t.Fatalf("ClientUserIDs after restrict = %v, want [%v]", gotRestricted.ClientUserIDs, clientUserID)
	}

	shared, err := store.SetBlockClients(ctx, userID, programID, week.ID, target, nil)
	if err != nil {
		t.Fatalf("SetBlockClients (clear): %v", err)
	}
	gotShared, ok := blockByID(shared, target)
	if !ok {
		t.Fatal("target block not found after clear")
	}
	if len(gotShared.ClientUserIDs) != 0 {
		t.Fatalf("ClientUserIDs after clearing = %v, want empty (delete-then-reinsert must not append)",
			gotShared.ClientUserIDs)
	}
}

// TestSetBlockClients_refusesLastSharedBlockInAssignedVersionOnly proves the
// version loop in ensureDaysKeepSharedBlock does real work: it constructs a
// case where the working copy alone would wrongly allow the restrict (it
// gained a third shared block after publish) while the frozen version a
// client is assigned to — which never saw that third block — would end up
// with zero shared blocks. Deleting the version loop must make this test
// fail.
func TestSetBlockClients_refusesLastSharedBlockInAssignedVersionOnly(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	store := program.NewStore(pool)

	userID, exerciseID := seedTrainerAndExercise(t, pool, "version-loop")
	detail, err := store.CreateDraft(ctx, userID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	day := week.Days[0]

	// Blocks A and B exist at publish time.
	for i := 0; i < 2; i++ {
		detail, err = createSingleBlockStore(ctx, store, userID, detail.ID, week.ID, day.ID,
			program.DayExerciseInput{ExerciseID: exerciseID})
		if err != nil {
			t.Fatalf("createSingleBlockStore %d: %v", i, err)
		}
	}
	blocks := detail.Weeks[0].Days[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks before publish = %d, want 2", len(blocks))
	}
	blockA, blockB := blocks[0], blocks[1]

	published, err := store.PublishFromDraft(ctx, detail.ID, userID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	clientUserID, trainerID := seedAssignedClient(t, pool, userID, "version-loop-client")
	programID := published.ID
	if _, err := store.SetClientProgramAssignment(ctx, userID, trainerID, clientUserID, &programID); err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	// Block C is added to the working copy AFTER publish: it exists there,
	// but not in the frozen version the client is assigned to.
	detail, err = createSingleBlockStore(ctx, store, userID, programID, week.ID, day.ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore (block C): %v", err)
	}
	if got := len(detail.Weeks[0].Days[0].Blocks); got != 3 {
		t.Fatalf("working copy blocks after adding C = %d, want 3", got)
	}

	// Restrict B: the working copy keeps A and C shared, and the assigned
	// version (which only ever had A and B) keeps A shared. Both pass.
	if _, err := store.SetBlockClients(ctx, userID, programID, week.ID, blockB.ID,
		[]uuid.UUID{clientUserID}); err != nil {
		t.Fatalf("SetBlockClients on B: %v", err)
	}

	// Restrict A too: the working copy still has C shared, so a check that
	// only looked at the working copy would pass. But the assigned version's
	// day only ever contained A and B — both now restricted — leaving it
	// with zero shared blocks. Only the per-version loop catches this.
	_, err = store.SetBlockClients(ctx, userID, programID, week.ID, blockA.ID,
		[]uuid.UUID{clientUserID})
	if !errors.Is(err, program.ErrLastSharedBlock) {
		t.Fatalf("SetBlockClients on A error = %v, want ErrLastSharedBlock", err)
	}
}
