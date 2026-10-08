//go:build integration

package storetest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
)

// assertModifiedBy reads modified_by of every row the query returns (one
// column) and requires at least one row, all of them written by want.
func assertModifiedBy(t *testing.T, pool *pgxpool.Pool, want uuid.UUID, query string, arg uuid.UUID) {
	t.Helper()
	rows, err := pool.Query(context.Background(), query, arg)
	if err != nil {
		t.Fatalf("query modified_by: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var got pgtype.UUID
		if err := rows.Scan(&got); err != nil {
			t.Fatalf("scan modified_by: %v", err)
		}
		n++
		if !got.Valid || uuid.UUID(got.Bytes) != want {
			t.Errorf("modified_by = %v (valid=%v), want %s", uuid.UUID(got.Bytes), got.Valid, want)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if n == 0 {
		t.Fatalf("no rows returned for %q", query)
	}
}

const (
	exercisesOfBlockQuery = `SELECT modified_by FROM mentorix.program_week_day_block_exercises WHERE program_week_day_block_id = $1`
	blocksOfDayQuery      = `SELECT modified_by FROM mentorix.program_week_day_blocks WHERE program_week_day_id = $1`
	weekQuery             = `SELECT modified_by FROM mentorix.program_weeks WHERE id = $1`
)

// TestProgramStore_mutatorsRecordActingUser creates every row as one trainer
// and mutates it as another, so a NULL or stale modified_by cannot match.
func TestProgramStore_mutatorsRecordActingUser(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	creator, err := authStore.RegisterTrainerEmailPassword(ctx, "audit-creator@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register creator: %v", err)
	}
	editor, err := authStore.RegisterTrainerEmailPassword(ctx, "audit-editor@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register editor: %v", err)
	}

	ex, err := exercise.NewStore(pool).Create(ctx, creator, nil, exercise.UpsertInput{
		Name: "A", NameRu: "А", Type: exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest, Difficulty: exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	store := program.NewStore(pool)
	sets, reps := "3", "10"
	in := program.DayExerciseInput{ExerciseID: ex.ID, Sets: &sets, Reps: &reps}

	type fixture struct {
		owner, programID, weekID, dayID uuid.UUID
	}
	// Each fixture gets its own owner: the free plan caps programs per trainer.
	newDraft := func(t *testing.T) fixture {
		t.Helper()
		owner, err := authStore.RegisterTrainerEmailPassword(ctx, uuid.NewString()+"@test.com", pwHash, "")
		if err != nil {
			t.Fatalf("register owner: %v", err)
		}
		d, err := store.CreateDraft(ctx, owner)
		if err != nil {
			t.Fatalf("CreateDraft: %v", err)
		}
		return fixture{owner: owner, programID: d.ID, weekID: d.Weeks[0].ID, dayID: d.Weeks[0].Days[0].ID}
	}
	dayBlocks := func(t *testing.T, f fixture) []program.DayBlock {
		t.Helper()
		d, err := store.GetDetail(ctx, f.programID)
		if err != nil {
			t.Fatalf("GetDetail: %v", err)
		}
		day, ok := findDay(d.Weeks[0], f.dayID)
		if !ok {
			t.Fatal("day not found")
		}
		return day.Blocks
	}
	addSingles := func(t *testing.T, f fixture, n int) []uuid.UUID {
		t.Helper()
		for i := 0; i < n; i++ {
			if _, err := createSingleBlockStore(ctx, store, f.owner, f.programID, f.weekID, f.dayID, in); err != nil {
				t.Fatalf("create single block: %v", err)
			}
		}
		blocks := dayBlocks(t, f)
		ids := make([]uuid.UUID, 0, len(blocks))
		for _, b := range blocks {
			ids = append(ids, b.ID)
		}
		return ids
	}
	// newGroup builds a group of n exercises plus one extra single block,
	// all written by the owner.
	newGroup := func(t *testing.T, n int) (fixture, program.DayBlock, uuid.UUID) {
		t.Helper()
		f := newDraft(t)
		ids := addSingles(t, f, n+1)
		if _, err := store.MergeDayBlocks(ctx, f.owner, f.programID, f.weekID, f.dayID, ids[:n]); err != nil {
			t.Fatalf("MergeDayBlocks: %v", err)
		}
		var group program.DayBlock
		var single uuid.UUID
		for _, b := range dayBlocks(t, f) {
			if b.BlockType == program.BlockTypeSingle {
				single = b.ID
			} else {
				group = b
			}
		}
		if group.ID == uuid.Nil || single == uuid.Nil {
			t.Fatal("expected one group and one single block")
		}
		return f, group, single
	}

	t.Run("AddWeek", func(t *testing.T) {
		f := newDraft(t)
		d, err := store.AddWeek(ctx, editor, f.programID)
		if err != nil {
			t.Fatalf("AddWeek: %v", err)
		}
		assertModifiedBy(t, pool, editor, weekQuery, d.Weeks[len(d.Weeks)-1].ID)
	})

	t.Run("AddBlockExercise", func(t *testing.T) {
		f, group, _ := newGroup(t, 2)
		if _, err := store.AddBlockExercise(ctx, editor, f.programID, f.weekID, group.ID, in); err != nil {
			t.Fatalf("AddBlockExercise: %v", err)
		}
		assertModifiedBy(t, pool, editor, exercisesOfBlockQuery, group.ID)
	})

	t.Run("DeleteBlockExercise from group", func(t *testing.T) {
		f, group, _ := newGroup(t, 3)
		if _, err := store.DeleteBlockExercise(ctx, editor, f.programID, f.weekID, group.ID, group.Exercises[0].ID); err != nil {
			t.Fatalf("DeleteBlockExercise: %v", err)
		}
		assertModifiedBy(t, pool, editor, exercisesOfBlockQuery, group.ID)
	})

	t.Run("DeleteBlockExercise from single", func(t *testing.T) {
		f, _, single := newGroup(t, 2)
		d, err := store.GetDetail(ctx, f.programID)
		if err != nil {
			t.Fatalf("GetDetail: %v", err)
		}
		day, _ := findDay(d.Weeks[0], f.dayID)
		var itemID uuid.UUID
		for _, b := range day.Blocks {
			if b.ID == single {
				itemID = b.Exercises[0].ID
			}
		}
		if _, err := store.DeleteBlockExercise(ctx, editor, f.programID, f.weekID, single, itemID); err != nil {
			t.Fatalf("DeleteBlockExercise: %v", err)
		}
		// The surviving group block is renumbered by the delete.
		assertModifiedBy(t, pool, editor, blocksOfDayQuery, f.dayID)
	})

	t.Run("DeleteDayBlock", func(t *testing.T) {
		f, group, _ := newGroup(t, 2)
		if _, err := store.DeleteDayBlock(ctx, editor, f.programID, f.weekID, group.ID); err != nil {
			t.Fatalf("DeleteDayBlock: %v", err)
		}
		assertModifiedBy(t, pool, editor, blocksOfDayQuery, f.dayID)
	})

	t.Run("MergeDayBlocks", func(t *testing.T) {
		f := newDraft(t)
		ids := addSingles(t, f, 2)
		if _, err := store.MergeDayBlocks(ctx, editor, f.programID, f.weekID, f.dayID, ids); err != nil {
			t.Fatalf("MergeDayBlocks: %v", err)
		}
		blocks := dayBlocks(t, f)
		if len(blocks) != 1 {
			t.Fatalf("blocks = %d, want 1", len(blocks))
		}
		assertModifiedBy(t, pool, editor, exercisesOfBlockQuery, blocks[0].ID)
		assertModifiedBy(t, pool, editor, blocksOfDayQuery, f.dayID)
	})

	t.Run("UngroupDayBlock", func(t *testing.T) {
		f, group, untouched := newGroup(t, 2)
		if _, err := store.UngroupDayBlock(ctx, editor, f.programID, f.weekID, group.ID); err != nil {
			t.Fatalf("UngroupDayBlock: %v", err)
		}
		for _, b := range dayBlocks(t, f) {
			if b.ID != untouched {
				assertModifiedBy(t, pool, editor, exercisesOfBlockQuery, b.ID)
			}
		}
	})

	t.Run("ExtractBlockExercise", func(t *testing.T) {
		f, group, _ := newGroup(t, 3)
		if _, err := store.ExtractBlockExercise(ctx, editor, f.programID, f.weekID, group.ID, group.Exercises[0].ID, 1); err != nil {
			t.Fatalf("ExtractBlockExercise: %v", err)
		}
		assertModifiedBy(t, pool, editor, exercisesOfBlockQuery, group.ID)
	})

	t.Run("MoveExerciseToBlock", func(t *testing.T) {
		f, group, _ := newGroup(t, 3)
		before := map[uuid.UUID]bool{}
		for _, b := range dayBlocks(t, f) {
			before[b.ID] = true
		}
		var fresh []uuid.UUID
		for _, id := range addSingles(t, f, 2) {
			if !before[id] {
				fresh = append(fresh, id)
			}
		}
		if _, err := store.MergeDayBlocks(ctx, f.owner, f.programID, f.weekID, f.dayID, fresh); err != nil {
			t.Fatalf("merge target group: %v", err)
		}
		var target uuid.UUID
		for _, b := range dayBlocks(t, f) {
			if !before[b.ID] && b.BlockType != program.BlockTypeSingle {
				target = b.ID
			}
		}
		if target == uuid.Nil {
			t.Fatal("target group not found")
		}
		if _, err := store.MoveExerciseToBlock(ctx, editor, f.programID, f.weekID, group.ID, group.Exercises[0].ID, target); err != nil {
			t.Fatalf("MoveExerciseToBlock: %v", err)
		}
		assertModifiedBy(t, pool, editor, exercisesOfBlockQuery, group.ID)
		assertModifiedBy(t, pool, editor, exercisesOfBlockQuery, target)
	})
}
