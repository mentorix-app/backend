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
