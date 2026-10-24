//go:build integration

package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/program"
)

type settingsFixture struct {
	pool      *pgxpool.Pool
	store     *program.Store
	trainer   uuid.UUID
	exercise  uuid.UUID
	programID uuid.UUID
	weekID    uuid.UUID
	dayID     uuid.UUID
}

func newSettingsFixture(t *testing.T, prefix string) *settingsFixture {
	t.Helper()
	pool := NewPool(t)
	store := program.NewStore(pool)
	trainer, exerciseID := seedTrainerAndExercise(t, pool, prefix)
	draft, err := store.CreateDraft(context.Background(), trainer)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	return &settingsFixture{
		pool:      pool,
		store:     store,
		trainer:   trainer,
		exercise:  exerciseID,
		programID: draft.ID,
		weekID:    draft.Weeks[0].ID,
		dayID:     draft.Weeks[0].Days[0].ID,
	}
}

func (f *settingsFixture) detail(t *testing.T) program.Detail {
	t.Helper()
	d, err := f.store.GetDetail(context.Background(), f.programID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	return d
}

func (f *settingsFixture) block(t *testing.T, id uuid.UUID) program.DayBlock {
	t.Helper()
	for _, w := range f.detail(t).Weeks {
		for _, d := range w.Days {
			for _, b := range d.Blocks {
				if b.ID == id {
					return b
				}
			}
		}
	}
	t.Fatalf("block %s not found", id)
	return program.DayBlock{}
}

// addSingle adds a single block with one exercise and returns its block.
func (f *settingsFixture) addSingle(t *testing.T, sets, reps string) program.DayBlock {
	t.Helper()
	in := program.DayExerciseInput{ExerciseID: f.exercise, Sets: &sets, Reps: &reps}
	in.NormalizeVolume()
	d, err := createSingleBlockStore(context.Background(), f.store, f.trainer, f.programID, f.weekID, f.dayID, in)
	if err != nil {
		t.Fatalf("create single block: %v", err)
	}
	day, _ := findDay(d.Weeks[0], f.dayID)
	return day.Blocks[len(day.Blocks)-1]
}

// addGroup merges two singles (sets "3", reps "10") into a group block and returns it.
func (f *settingsFixture) addGroup(t *testing.T) program.DayBlock {
	t.Helper()
	a := f.addSingle(t, "3", "10")
	b := f.addSingle(t, "3", "10")
	if _, err := f.store.MergeDayBlocks(context.Background(), f.trainer, f.programID, f.weekID, f.dayID, []uuid.UUID{a.ID, b.ID}); err != nil {
		t.Fatalf("MergeDayBlocks: %v", err)
	}
	// The first block of the merge survives as the group.
	return f.block(t, a.ID)
}

func (f *settingsFixture) patch(blockID uuid.UUID, in program.BlockPatchInput) (program.Detail, error) {
	return f.store.PatchDayBlock(context.Background(), f.trainer, f.programID, f.weekID, blockID, in)
}

func setInt(v int) program.OptionalInt { return program.OptionalInt{Present: true, Value: &v} }

func nullInt() program.OptionalInt { return program.OptionalInt{Present: true} }

func blockType(bt program.BlockType) *program.BlockType { return &bt }

func intEq(got *int, want *int) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func ip(v int) *int { return &v }

func assertSettings(t *testing.T, b program.DayBlock, rounds, duration, timeCap, work, rest *int) {
	t.Helper()
	if !intEq(b.Rounds, rounds) || !intEq(b.DurationSec, duration) || !intEq(b.TimeCapSec, timeCap) ||
		!intEq(b.WorkSec, work) || !intEq(b.RestSec, rest) {
		t.Fatalf("settings (rounds,duration,time_cap,work,rest) = (%s,%s,%s,%s,%s), want (%s,%s,%s,%s,%s)",
			fmtInt(b.Rounds), fmtInt(b.DurationSec), fmtInt(b.TimeCapSec), fmtInt(b.WorkSec), fmtInt(b.RestSec),
			fmtInt(rounds), fmtInt(duration), fmtInt(timeCap), fmtInt(work), fmtInt(rest))
	}
}

func fmtInt(v *int) string {
	if v == nil {
		return "nil"
	}
	b, _ := json.Marshal(*v)
	return string(b)
}

func TestMerge_createsSkillWorkWithoutSettingsAndKeepsSets(t *testing.T) {
	f := newSettingsFixture(t, "settings-merge")
	group := f.addGroup(t)

	if group.BlockType != program.BlockTypeSkillWork {
		t.Fatalf("merged block_type = %s, want skill_work", group.BlockType)
	}
	assertSettings(t, group, nil, nil, nil, nil, nil)
	if len(group.Exercises) != 2 {
		t.Fatalf("exercises = %d, want 2", len(group.Exercises))
	}
	for _, ex := range group.Exercises {
		if ex.Sets == nil || *ex.Sets != "3" {
			t.Fatalf("merged exercise sets = %v, want 3 (skill_work keeps sets)", ex.Sets)
		}
	}
}

func TestPatchDayBlock_settingLifecycle(t *testing.T) {
	f := newSettingsFixture(t, "settings-lifecycle")
	group := f.addGroup(t)

	steps := []struct {
		name    string
		in      program.BlockPatchInput
		wantErr bool
		wantTyp program.BlockType
		rounds  *int
		dur     *int
		cap     *int
		work    *int
		rest    *int
	}{
		{"emom with rounds", program.BlockPatchInput{BlockType: blockType(program.BlockTypeEMOM), Rounds: setInt(8)}, false, program.BlockTypeEMOM, ip(8), nil, nil, nil, nil},
		{"absent keeps rounds", program.BlockPatchInput{Instruction: strPtr("go")}, false, program.BlockTypeEMOM, ip(8), nil, nil, nil, nil},
		{"for_time carries rounds and takes the cap", program.BlockPatchInput{BlockType: blockType(program.BlockTypeForTime), TimeCapSec: setInt(600)}, false, program.BlockTypeForTime, ip(8), nil, ip(600), nil, nil},
		{"null clears rounds", program.BlockPatchInput{Rounds: nullInt()}, false, program.BlockTypeForTime, nil, nil, ip(600), nil, nil},
		{"duration refused for for_time", program.BlockPatchInput{DurationSec: setInt(60)}, true, program.BlockTypeForTime, nil, nil, ip(600), nil, nil},
		{"intervals resets the cap and keeps zero rest", program.BlockPatchInput{BlockType: blockType(program.BlockTypeIntervals), Rounds: setInt(4), WorkSec: setInt(30), RestSec: setInt(0)}, false, program.BlockTypeIntervals, ip(4), nil, nil, ip(30), ip(0)},
		{"superset keeps rounds and resets work and rest", program.BlockPatchInput{BlockType: blockType(program.BlockTypeSuperset)}, false, program.BlockTypeSuperset, ip(4), nil, nil, nil, nil},
		{"amrap resets rounds", program.BlockPatchInput{BlockType: blockType(program.BlockTypeAMRAP), DurationSec: setInt(900)}, false, program.BlockTypeAMRAP, nil, ip(900), nil, nil, nil},
		{"rounds refused for amrap", program.BlockPatchInput{Rounds: setInt(3)}, true, program.BlockTypeAMRAP, nil, ip(900), nil, nil, nil},
		{"null for a not allowed field is fine", program.BlockPatchInput{Rounds: nullInt()}, false, program.BlockTypeAMRAP, nil, ip(900), nil, nil, nil},
		{"chipper resets everything", program.BlockPatchInput{BlockType: blockType(program.BlockTypeChipper)}, false, program.BlockTypeChipper, nil, nil, nil, nil, nil},
		{"strength takes nothing", program.BlockPatchInput{BlockType: blockType(program.BlockTypeStrength), Rounds: setInt(2)}, true, program.BlockTypeChipper, nil, nil, nil, nil, nil},
	}
	for _, s := range steps {
		_, err := f.patch(group.ID, s.in)
		if s.wantErr {
			if !errors.Is(err, program.ErrValidation) {
				t.Fatalf("%s: err = %v, want ErrValidation", s.name, err)
			}
		} else if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		b := f.block(t, group.ID)
		if b.BlockType != s.wantTyp {
			t.Fatalf("%s: block_type = %s, want %s", s.name, b.BlockType, s.wantTyp)
		}
		assertSettings(t, b, s.rounds, s.dur, s.cap, s.work, s.rest)
	}
}

func strPtr(s string) *string { return &s }

func TestPatchDayBlock_refusedPatchChangesNothing(t *testing.T) {
	f := newSettingsFixture(t, "settings-atomic")
	group := f.addGroup(t)

	// The type change would clear sets, but the request is refused as a whole.
	_, err := f.patch(group.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeEMOM), DurationSec: setInt(60)})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	b := f.block(t, group.ID)
	if b.BlockType != program.BlockTypeSkillWork {
		t.Fatalf("block_type = %s, want skill_work", b.BlockType)
	}
	for _, ex := range b.Exercises {
		if ex.Sets == nil {
			t.Fatal("sets were cleared by a refused patch")
		}
	}
}

func TestPatchDayBlock_typeChangeClearsSetsOnlyForTypesWithoutSets(t *testing.T) {
	f := newSettingsFixture(t, "settings-sets-clear")
	group := f.addGroup(t)

	// skill_work -> strength: both carry sets.
	if _, err := f.patch(group.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeStrength)}); err != nil {
		t.Fatalf("patch strength: %v", err)
	}
	for _, ex := range f.block(t, group.ID).Exercises {
		if ex.Sets == nil || *ex.Sets != "3" {
			t.Fatalf("sets = %v, want 3 kept", ex.Sets)
		}
	}

	// strength -> emom: sets go, reps stay.
	if _, err := f.patch(group.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeEMOM)}); err != nil {
		t.Fatalf("patch emom: %v", err)
	}
	for _, ex := range f.block(t, group.ID).Exercises {
		if ex.Sets != nil {
			t.Fatalf("sets = %q, want cleared", *ex.Sets)
		}
		if ex.Reps == nil || *ex.Reps != "10" {
			t.Fatalf("reps = %v, want 10 kept", ex.Reps)
		}
	}
}

func TestPatchDayBlock_clearsStraySetsWithoutATypeChange(t *testing.T) {
	f := newSettingsFixture(t, "settings-stray-sets")
	ctx := context.Background()
	group := f.addGroup(t)
	if _, err := f.patch(group.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeEMOM), Rounds: setInt(3)}); err != nil {
		t.Fatalf("patch emom: %v", err)
	}

	// Old code can still write sets into an emom block during the deploy window.
	if _, err := f.pool.Exec(ctx,
		`UPDATE mentorix.program_week_day_block_exercises SET sets = '5' WHERE program_week_day_block_id = $1`,
		group.ID); err != nil {
		t.Fatalf("simulate old code: %v", err)
	}
	for _, ex := range f.block(t, group.ID).Exercises {
		if ex.Sets == nil {
			t.Fatal("test setup: stray sets were not written")
		}
	}

	// A PATCH that keeps the type emom still cleans them.
	if _, err := f.patch(group.ID, program.BlockPatchInput{Rounds: setInt(4)}); err != nil {
		t.Fatalf("patch rounds: %v", err)
	}
	b := f.block(t, group.ID)
	if b.BlockType != program.BlockTypeEMOM {
		t.Fatalf("block_type = %s, want emom", b.BlockType)
	}
	for _, ex := range b.Exercises {
		if ex.Sets != nil {
			t.Fatalf("exercise %s kept stray sets %q", ex.ID, *ex.Sets)
		}
	}
}

func TestPatchDayBlock_keepsSetsInBlockThatCarriesThem(t *testing.T) {
	f := newSettingsFixture(t, "settings-keep-sets")
	group := f.addGroup(t)
	if _, err := f.patch(group.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeStrength)}); err != nil {
		t.Fatalf("patch strength: %v", err)
	}

	instruction := "heavy"
	if _, err := f.patch(group.ID, program.BlockPatchInput{Instruction: &instruction}); err != nil {
		t.Fatalf("patch instruction: %v", err)
	}
	for _, ex := range f.block(t, group.ID).Exercises {
		if ex.Sets == nil || *ex.Sets != "3" {
			t.Fatalf("sets = %v, want 3 kept", ex.Sets)
		}
	}
}

func TestBlockExercise_setsRefusedInBlockWithoutSets(t *testing.T) {
	f := newSettingsFixture(t, "settings-exercise-sets")
	group := f.addGroup(t)
	ctx := context.Background()
	if _, err := f.patch(group.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeEMOM)}); err != nil {
		t.Fatalf("patch emom: %v", err)
	}
	sets, reps := "4", "8"

	_, err := f.store.AddBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID,
		program.DayExerciseInput{ExerciseID: f.exercise, Sets: &sets, Reps: &reps})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("add with sets: err = %v, want ErrValidation", err)
	}
	if got := len(f.block(t, group.ID).Exercises); got != 2 {
		t.Fatalf("exercises = %d, want 2 after the refused add", got)
	}

	if _, err := f.store.AddBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID,
		program.DayExerciseInput{ExerciseID: f.exercise, Reps: &reps}); err != nil {
		t.Fatalf("add without sets: %v", err)
	}

	item := f.block(t, group.ID).Exercises[0]
	_, err = f.store.UpdateBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID, item.ID,
		program.DayExerciseInput{ExerciseID: item.ExerciseID, Sets: &sets, Reps: &reps})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("update with sets: err = %v, want ErrValidation", err)
	}
	if _, err := f.store.UpdateBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID, item.ID,
		program.DayExerciseInput{ExerciseID: item.ExerciseID, Reps: &reps}); err != nil {
		t.Fatalf("update without sets: %v", err)
	}
	updated := f.block(t, group.ID).Exercises[0]
	if updated.Sets != nil || updated.Reps == nil || *updated.Reps != "8" {
		t.Fatalf("updated exercise = sets %v reps %v, want no sets and reps 8", updated.Sets, updated.Reps)
	}
}

func TestBlockExercise_setsAllowedWhereTheTypeCarriesThem(t *testing.T) {
	f := newSettingsFixture(t, "settings-exercise-ok")
	group := f.addGroup(t)
	ctx := context.Background()
	sets, reps := "4", "8"

	if _, err := f.store.AddBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID,
		program.DayExerciseInput{ExerciseID: f.exercise, Sets: &sets, Reps: &reps}); err != nil {
		t.Fatalf("add with sets to skill_work: %v", err)
	}
	single := f.addSingle(t, "5", "5")
	item := single.Exercises[0]
	if _, err := f.store.UpdateBlockExercise(ctx, f.trainer, f.programID, f.weekID, single.ID, item.ID,
		program.DayExerciseInput{ExerciseID: item.ExerciseID, Sets: &sets, Reps: &reps}); err != nil {
		t.Fatalf("update sets of a single: %v", err)
	}
	if got := f.block(t, single.ID).Exercises[0].Sets; got == nil || *got != "4" {
		t.Fatalf("single sets = %v, want 4", got)
	}
}

func TestMoveExerciseToBlock_clearsSetsOnlyInBlockWithoutSets(t *testing.T) {
	f := newSettingsFixture(t, "settings-move")
	ctx := context.Background()
	timed := f.addGroup(t)
	if _, err := f.patch(timed.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeEMOM)}); err != nil {
		t.Fatalf("patch emom: %v", err)
	}
	withSets := f.addGroup(t)

	toTimed := f.addSingle(t, "3", "10")
	if _, err := f.store.MoveExerciseToBlock(ctx, f.trainer, f.programID, f.weekID, toTimed.ID, toTimed.Exercises[0].ID, timed.ID); err != nil {
		t.Fatalf("move into emom: %v", err)
	}
	moved := f.block(t, timed.ID).Exercises
	if len(moved) != 3 {
		t.Fatalf("emom exercises = %d, want 3", len(moved))
	}
	for _, ex := range moved {
		if ex.Sets != nil {
			t.Fatalf("exercise in emom has sets %q", *ex.Sets)
		}
		if ex.Reps == nil || *ex.Reps != "10" {
			t.Fatalf("reps = %v, want 10 kept", ex.Reps)
		}
	}

	toSkill := f.addSingle(t, "3", "10")
	if _, err := f.store.MoveExerciseToBlock(ctx, f.trainer, f.programID, f.weekID, toSkill.ID, toSkill.Exercises[0].ID, withSets.ID); err != nil {
		t.Fatalf("move into skill_work: %v", err)
	}
	for _, ex := range f.block(t, withSets.ID).Exercises {
		if ex.Sets == nil || *ex.Sets != "3" {
			t.Fatalf("exercise in skill_work sets = %v, want 3", ex.Sets)
		}
	}
}

// lockProgramInTx opens a transaction that holds the program row lock until it ends.
func lockProgramInTx(t *testing.T, pool *pgxpool.Pool, programID uuid.UUID) pgx.Tx {
	t.Helper()
	ctx := context.Background()
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = holder.Rollback(ctx) })
	if _, err := holder.Exec(ctx, `SELECT id FROM mentorix.programs WHERE id = $1 FOR NO KEY UPDATE`, programID); err != nil {
		t.Fatalf("lock program: %v", err)
	}
	return holder
}

// waitForProgramLockWaiter blocks until a session is queued behind the lock the holder
// transaction holds, so a test can change data after the waiter passed its unlocked checks and
// before it gets the lock. The holder locks only one program row, so a backend blocked by it
// waits for that program and no other.
func waitForProgramLockWaiter(t *testing.T, pool *pgxpool.Pool, holder pgx.Tx) {
	t.Helper()
	ctx := context.Background()
	var holderPID int
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatalf("holder pid: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(ctx, `
SELECT count(*) FROM pg_stat_activity
WHERE datname = current_database()
  AND pid <> $1
  AND wait_event_type = 'Lock'
  AND $1 = ANY (pg_blocking_pids(pid))`, holderPID).Scan(&waiting); err != nil {
			t.Fatalf("poll lock waiters: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no session waited for the program lock")
}

func TestMoveExerciseToBlock_targetDeletedBeforeTheLockIsNotFound(t *testing.T) {
	f := newSettingsFixture(t, "settings-move-gone")
	ctx := context.Background()
	source := f.addSingle(t, "3", "10")
	target := f.addGroup(t)

	// Hold the program lock, let the move pass its unlocked checks and queue behind it,
	// then delete the target before releasing.
	holder := lockProgramInTx(t, f.pool, f.programID)

	moveErr := make(chan error, 1)
	go func() {
		_, err := f.store.MoveExerciseToBlock(ctx, f.trainer, f.programID, f.weekID, source.ID, source.Exercises[0].ID, target.ID)
		moveErr <- err
	}()
	waitForProgramLockWaiter(t, f.pool, holder)

	if _, err := holder.Exec(ctx, `DELETE FROM mentorix.program_week_day_blocks WHERE id = $1`, target.ID); err != nil {
		t.Fatalf("delete target: %v", err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := <-moveErr; !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("err = %v, want pgx.ErrNoRows (not found)", err)
	}
	if got := f.block(t, source.ID).Exercises; len(got) != 1 || got[0].ID != source.Exercises[0].ID {
		t.Fatalf("source block exercises = %+v, want the exercise untouched", got)
	}
}

func TestAddBlockExercise_queuedBehindATypeChangeIsRefused(t *testing.T) {
	f := newSettingsFixture(t, "settings-race")
	group := f.addGroup(t)
	ctx := context.Background()
	sets, reps := "4", "8"

	// The add passes its unlocked checks while the block still carries sets, queues on the
	// program lock, and only gets it after the block became an emom.
	holder := lockProgramInTx(t, f.pool, f.programID)
	addErr := make(chan error, 1)
	go func() {
		_, err := f.store.AddBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID,
			program.DayExerciseInput{ExerciseID: f.exercise, Sets: &sets, Reps: &reps})
		addErr <- err
	}()
	waitForProgramLockWaiter(t, f.pool, holder)

	if _, err := holder.Exec(ctx,
		`UPDATE mentorix.program_week_day_blocks SET block_type = 'emom' WHERE id = $1`, group.ID); err != nil {
		t.Fatalf("change type: %v", err)
	}
	if _, err := holder.Exec(ctx,
		`UPDATE mentorix.program_week_day_block_exercises SET sets = NULL WHERE program_week_day_block_id = $1`,
		group.ID); err != nil {
		t.Fatalf("clear sets: %v", err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := <-addErr; !errors.Is(err, program.ErrValidation) {
		t.Fatalf("AddBlockExercise err = %v, want ErrValidation", err)
	}
	b := f.block(t, group.ID)
	if b.BlockType != program.BlockTypeEMOM {
		t.Fatalf("block_type = %s, want emom", b.BlockType)
	}
	if len(b.Exercises) != 2 {
		t.Fatalf("exercises = %d, want 2 after the refused add", len(b.Exercises))
	}
	for _, ex := range b.Exercises {
		if ex.Sets != nil {
			t.Fatalf("exercise %s in an emom block has sets %q", ex.ID, *ex.Sets)
		}
	}
}

func TestBlockSettings_surviveDetailPublishAndDiscard(t *testing.T) {
	f := newSettingsFixture(t, "settings-publish")
	ctx := context.Background()
	group := f.addGroup(t)
	if _, err := f.patch(group.ID, program.BlockPatchInput{
		BlockType: blockType(program.BlockTypeIntervals),
		Rounds:    setInt(6), WorkSec: setInt(40), RestSec: setInt(0),
	}); err != nil {
		t.Fatalf("patch intervals: %v", err)
	}
	assertSettings(t, f.block(t, group.ID), ip(6), nil, nil, ip(40), ip(0))

	// The frozen version carries the settings, rest_sec 0 included.
	detail := f.detail(t)
	published, err := f.store.PublishFromDraft(ctx, f.programID, f.trainer, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	versionID := *published.LatestProgramVersionID
	version, err := f.store.GetVersionDetail(ctx, versionID)
	if err != nil {
		t.Fatalf("GetVersionDetail: %v", err)
	}
	var frozen program.DayBlock
	for _, b := range version.Weeks[0].Days[0].Blocks {
		if b.BlockKey == group.BlockKey {
			frozen = b
		}
	}
	if frozen.ID == uuid.Nil {
		t.Fatal("frozen block not found")
	}
	assertSettings(t, frozen, ip(6), nil, nil, ip(40), ip(0))

	// Edit the working copy, then discard: the version values come back.
	if _, err := f.patch(group.ID, program.BlockPatchInput{BlockType: blockType(program.BlockTypeAMRAP), DurationSec: setInt(300)}); err != nil {
		t.Fatalf("patch amrap: %v", err)
	}
	restored, err := f.store.RestoreWorkingTreeFromLatestVersion(ctx, f.programID, f.trainer)
	if err != nil {
		t.Fatalf("RestoreWorkingTreeFromLatestVersion: %v", err)
	}
	var back program.DayBlock
	for _, b := range restored.Weeks[0].Days[0].Blocks {
		if b.BlockKey == group.BlockKey {
			back = b
		}
	}
	if back.BlockType != program.BlockTypeIntervals {
		t.Fatalf("restored block_type = %s, want intervals", back.BlockType)
	}
	assertSettings(t, back, ip(6), nil, nil, ip(40), ip(0))
	assertSettings(t, f.block(t, back.ID), ip(6), nil, nil, ip(40), ip(0))
}

func TestBlockSettings_jsonAlwaysCarriesTheKeys(t *testing.T) {
	f := newSettingsFixture(t, "settings-json")
	group := f.addGroup(t)

	raw, err := json.Marshal(f.block(t, group.ID))
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"rounds", "duration_sec", "time_cap_sec", "work_sec", "rest_sec"} {
		v, ok := keys[k]
		if !ok || strings.TrimSpace(string(v)) != "null" {
			t.Fatalf("%s = %s (present %v), want null", k, v, ok)
		}
	}
}

func TestBlockSettings_databaseRefusesOutOfRangeValues(t *testing.T) {
	f := newSettingsFixture(t, "settings-check")
	group := f.addGroup(t)
	ctx := context.Background()

	for _, tc := range []struct{ column, value string }{
		{"rounds", "0"}, {"rounds", "101"},
		{"duration_sec", "0"}, {"duration_sec", "86401"},
		{"time_cap_sec", "0"}, {"work_sec", "0"},
		{"rest_sec", "-1"}, {"rest_sec", "86401"},
	} {
		_, err := f.pool.Exec(ctx, `UPDATE mentorix.program_week_day_blocks SET `+tc.column+` = `+tc.value+` WHERE id = $1`, group.ID)
		if err == nil {
			t.Fatalf("%s = %s was accepted", tc.column, tc.value)
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE mentorix.program_week_day_blocks SET rest_sec = 0, rounds = 100 WHERE id = $1`, group.ID); err != nil {
		t.Fatalf("boundary values refused: %v", err)
	}
}
