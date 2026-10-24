//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
)

// The copy and clear queries carry their own scope, so a wrong id changes nothing even if a
// caller skips its checks. The store validates ids first, so these call the queries directly.

func TestInsertDayBlockCopy_sourceOfAnotherProgramInsertsNothing(t *testing.T) {
	mine := newSettingsFixture(t, "scope-block")
	ctx := context.Background()
	other, err := mine.store.CreateDraft(ctx, mine.trainer)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	sets, reps := "3", "10"
	withBlock, err := createSingleBlockStore(ctx, mine.store, mine.trainer, other.ID, other.Weeks[0].ID, other.Weeks[0].Days[0].ID,
		program.DayExerciseInput{ExerciseID: mine.exercise, Sets: &sets, Reps: &reps})
	if err != nil {
		t.Fatalf("create block in the other program: %v", err)
	}
	foreign := withBlock.Weeks[0].Days[0].Blocks[0]

	_, err = sqlc.New(mine.pool).InsertDayBlockCopy(ctx, sqlc.InsertDayBlockCopyParams{
		ProgramWeekDayID: pgconv.ToPGUUID(mine.dayID),
		SortOrder:        1,
		ModifiedAt:       time.Now().UTC(),
		ModifiedBy:       pgconv.ToPGUUID(mine.trainer),
		ProgramID:        pgconv.ToPGUUID(mine.programID),
		SourceBlockID:    pgconv.ToPGUUID(foreign.ID),
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("err = %v, want pgx.ErrNoRows", err)
	}
	if n := countRows(t, mine.pool, `SELECT count(*) FROM mentorix.program_week_day_blocks WHERE program_week_day_id = $1`, mine.dayID); n != 0 {
		t.Fatalf("blocks in the day = %d, want 0", n)
	}
}

func TestInsertBlockExerciseCopy_itemOfAnotherBlockInsertsNothing(t *testing.T) {
	f := newSettingsFixture(t, "scope-item")
	a := f.addSingle(t, "3", "10")
	b := f.addSingle(t, "5", "5")
	ctx := context.Background()

	_, err := sqlc.New(f.pool).InsertBlockExerciseCopy(ctx, sqlc.InsertBlockExerciseCopyParams{
		ProgramWeekDayBlockID: pgconv.ToPGUUID(a.ID),
		SortOrder:             2,
		ModifiedAt:            time.Now().UTC(),
		ModifiedBy:            pgconv.ToPGUUID(f.trainer),
		SourceBlockID:         pgconv.ToPGUUID(a.ID),
		SourceItemID:          pgconv.ToPGUUID(b.Exercises[0].ID),
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("err = %v, want pgx.ErrNoRows", err)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM mentorix.program_week_day_block_exercises WHERE program_week_day_block_id = $1`, a.ID); n != 1 {
		t.Fatalf("exercises in the block = %d, want 1", n)
	}
}

func TestClearBlockExerciseSetsByID_otherBlockChangesNothing(t *testing.T) {
	f := newSettingsFixture(t, "scope-clear")
	a := f.addSingle(t, "3", "10")
	b := f.addSingle(t, "5", "5")
	ctx := context.Background()

	if err := sqlc.New(f.pool).ClearBlockExerciseSetsByID(ctx, sqlc.ClearBlockExerciseSetsByIDParams{
		ID:                    pgconv.ToPGUUID(a.Exercises[0].ID),
		ModifiedAt:            time.Now().UTC(),
		ModifiedBy:            pgconv.ToPGUUID(f.trainer),
		ProgramWeekDayBlockID: pgconv.ToPGUUID(b.ID),
	}); err != nil {
		t.Fatalf("ClearBlockExerciseSetsByID: %v", err)
	}
	if got := f.block(t, a.ID).Exercises[0].Sets; got == nil || *got != "3" {
		t.Fatalf("sets = %v, want 3 kept", got)
	}
}

func TestClearBlockExerciseSets_leavesRowsWithoutSetsUntouched(t *testing.T) {
	f := newSettingsFixture(t, "scope-clear-null")
	group := f.addGroup(t)
	ctx := context.Background()
	if _, err := f.patch(group.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeEMOM)}); err != nil {
		t.Fatalf("patch emom: %v", err)
	}
	var before time.Time
	if err := f.pool.QueryRow(ctx, `SELECT max(modified_at) FROM mentorix.program_week_day_block_exercises WHERE program_week_day_block_id = $1`, group.ID).Scan(&before); err != nil {
		t.Fatalf("select modified_at: %v", err)
	}

	if err := sqlc.New(f.pool).ClearBlockExerciseSets(ctx, sqlc.ClearBlockExerciseSetsParams{
		ProgramWeekDayBlockID: pgconv.ToPGUUID(group.ID),
		ModifiedAt:            before.Add(time.Hour),
		ModifiedBy:            pgconv.ToPGUUID(f.trainer),
	}); err != nil {
		t.Fatalf("ClearBlockExerciseSets: %v", err)
	}
	if n := countRows(t, f.pool, `SELECT count(*) FROM mentorix.program_week_day_block_exercises WHERE program_week_day_block_id = $1 AND modified_at > $2`, group.ID, before); n != 0 {
		t.Fatalf("%d rows without sets were rewritten", n)
	}
}
