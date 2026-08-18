//go:build integration

package storetest

import (
	"context"
	"strings"
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
