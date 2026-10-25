//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/program"
)

func requireBlockSortOrders(t *testing.T, blocks []program.DayBlock) {
	t.Helper()
	for i, b := range blocks {
		if b.SortOrder != i+1 {
			t.Fatalf("block %d sort_order = %d, want %d", i, b.SortOrder, i+1)
		}
	}
}

func requireExerciseSortOrders(t *testing.T, exercises []program.DayExercise) {
	t.Helper()
	for i, ex := range exercises {
		if ex.SortOrder != i+1 {
			t.Fatalf("exercise %d sort_order = %d, want %d", i, ex.SortOrder, i+1)
		}
	}
}

func fmtStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return "=" + *s
}

// requireCopiedExercise checks that got is a new row carrying the values of want.
func requireCopiedExercise(t *testing.T, got, want program.DayExercise) {
	t.Helper()
	if got.ID == want.ID {
		t.Fatalf("copied exercise reuses id %s", got.ID)
	}
	if got.ExerciseID != want.ExerciseID || got.Instruction != want.Instruction ||
		fmtStr(got.Sets) != fmtStr(want.Sets) || fmtStr(got.Reps) != fmtStr(want.Reps) {
		t.Fatalf("copied exercise = %+v, want the values of %+v", got, want)
	}
}

func findBlock(t *testing.T, d program.Detail, id uuid.UUID) program.DayBlock {
	t.Helper()
	for _, w := range d.Weeks {
		for _, day := range w.Days {
			for _, b := range day.Blocks {
				if b.ID == id {
					return b
				}
			}
		}
	}
	t.Fatalf("block %s not found", id)
	return program.DayBlock{}
}

func requireClients(t *testing.T, b program.DayBlock, want ...uuid.UUID) {
	t.Helper()
	if len(b.ClientUserIDs) != len(want) {
		t.Fatalf("block %s client_user_ids = %v, want %v", b.ID, b.ClientUserIDs, want)
	}
	for i := range want {
		if b.ClientUserIDs[i] != want[i] {
			t.Fatalf("block %s client_user_ids = %v, want %v", b.ID, b.ClientUserIDs, want)
		}
	}
}

// publishedGroupFixture is a published program with a client, a day with four singles and
// a second week, so a copy can leave its source day and week.
type publishedGroupFixture struct {
	pool      *pgxpool.Pool
	store     *program.Store
	trainer   uuid.UUID
	client    uuid.UUID
	programID uuid.UUID
	weekID    uuid.UUID
	dayID     uuid.UUID
	blocks    []program.DayBlock
}

func newPublishedGroupFixture(t *testing.T, prefix string) *publishedGroupFixture {
	t.Helper()
	pool := NewPool(t)
	programID, weekID, blocks, client := seedDayWithBlocks(t, pool, prefix, 4)
	return &publishedGroupFixture{
		pool:      pool,
		store:     program.NewStore(pool),
		trainer:   ownerUserID(t, pool, programID),
		client:    client,
		programID: programID,
		weekID:    weekID,
		dayID:     dayIDOfBlock(t, pool, blocks[0].ID),
		blocks:    blocks,
	}
}

func (f *publishedGroupFixture) detail(t *testing.T) program.Detail {
	t.Helper()
	d, err := f.store.GetDetail(context.Background(), f.programID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	return d
}

// addWeek appends a week and returns it.
func (f *publishedGroupFixture) addWeek(t *testing.T) program.Week {
	t.Helper()
	d, err := f.store.AddWeek(context.Background(), f.trainer, f.programID)
	if err != nil {
		t.Fatalf("AddWeek: %v", err)
	}
	return d.Weeks[len(d.Weeks)-1]
}

// emomGroup merges the first two singles into an emom group with rounds 5, an instruction
// and an instruction on its first exercise.
func (f *publishedGroupFixture) emomGroup(t *testing.T) program.DayBlock {
	t.Helper()
	ctx := context.Background()
	if _, err := f.store.MergeDayBlocks(ctx, f.trainer, f.programID, f.weekID, f.dayID,
		[]uuid.UUID{f.blocks[0].ID, f.blocks[1].ID}); err != nil {
		t.Fatalf("MergeDayBlocks: %v", err)
	}
	emom, instruction := program.BlockTypeEMOM, "every minute"
	d, err := f.store.PatchDayBlock(ctx, f.trainer, f.programID, f.weekID, f.blocks[0].ID, program.BlockPatchInput{
		BlockType:   &emom,
		Instruction: &instruction,
		Rounds:      setInt(5),
	})
	if err != nil {
		t.Fatalf("PatchDayBlock: %v", err)
	}
	group := findBlock(t, d, f.blocks[0].ID)
	reps, note := "12", "light"
	if _, err := f.store.UpdateBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID, group.Exercises[0].ID,
		program.DayExerciseInput{ExerciseID: group.Exercises[0].ExerciseID, Reps: &reps, Instruction: &note}); err != nil {
		t.Fatalf("UpdateBlockExercise: %v", err)
	}
	return findBlock(t, f.detail(t), group.ID)
}

func TestDuplicateBlockExercise_groupInsertsRightAfterOriginal(t *testing.T) {
	f := newSettingsFixture(t, "dup-group")
	ctx := context.Background()
	group := f.addGroup(t)

	sets, reps, instruction := "4", "8", "slow tempo"
	if _, err := f.store.UpdateBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID, group.Exercises[0].ID,
		program.DayExerciseInput{ExerciseID: f.exercise, Sets: &sets, Reps: &reps, Instruction: &instruction}); err != nil {
		t.Fatalf("UpdateBlockExercise: %v", err)
	}
	group = f.block(t, group.ID)
	original := group.Exercises[0]

	d, err := f.store.DuplicateBlockExercise(ctx, f.trainer, f.programID, f.weekID, group.ID, original.ID)
	if err != nil {
		t.Fatalf("DuplicateBlockExercise: %v", err)
	}
	day, _ := findDay(d.Weeks[0], f.dayID)
	if len(day.Blocks) != 1 {
		t.Fatalf("blocks in day = %d, want 1", len(day.Blocks))
	}
	got := day.Blocks[0]
	if len(got.Exercises) != 3 {
		t.Fatalf("exercises = %d, want 3", len(got.Exercises))
	}
	requireExerciseSortOrders(t, got.Exercises)
	if got.Exercises[0].ID != original.ID || got.Exercises[2].ID != group.Exercises[1].ID {
		t.Fatalf("exercise order = %v, want original, copy, old second", got.Exercises)
	}
	requireCopiedExercise(t, got.Exercises[1], original)
}

func TestDuplicateBlockExercise_singleCreatesNewSingleRightAfter(t *testing.T) {
	f := newPublishedGroupFixture(t, "dup-single")
	ctx := context.Background()
	source := f.blocks[1]

	if _, err := f.store.SetBlockClients(ctx, f.trainer, f.programID, f.weekID, source.ID, []uuid.UUID{f.client}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}
	before := f.detail(t)
	original := findBlock(t, before, source.ID)

	d, err := f.store.DuplicateBlockExercise(ctx, f.trainer, f.programID, f.weekID, source.ID, original.Exercises[0].ID)
	if err != nil {
		t.Fatalf("DuplicateBlockExercise: %v", err)
	}
	day, _ := findDay(d.Weeks[0], f.dayID)
	if len(day.Blocks) != 5 {
		t.Fatalf("blocks in day = %d, want 5", len(day.Blocks))
	}
	requireBlockSortOrders(t, day.Blocks)
	if day.Blocks[1].ID != source.ID {
		t.Fatalf("original block moved: position 2 holds %s", day.Blocks[1].ID)
	}
	dup := day.Blocks[2]
	if dup.ID == source.ID || dup.BlockKey == original.BlockKey {
		t.Fatalf("duplicate must be a new block with a new block_key, got id=%s key=%s", dup.ID, dup.BlockKey)
	}
	if dup.BlockType != program.BlockTypeSingle || dup.Instruction != original.Instruction {
		t.Fatalf("duplicate = %s %q, want single %q", dup.BlockType, dup.Instruction, original.Instruction)
	}
	assertSettings(t, dup, nil, nil, nil, nil, nil)
	if len(dup.Exercises) != 1 {
		t.Fatalf("duplicate exercises = %d, want 1", len(dup.Exercises))
	}
	requireCopiedExercise(t, dup.Exercises[0], original.Exercises[0])
	requireClients(t, dup, f.client)
	if len(findBlock(t, d, source.ID).Exercises) != 1 {
		t.Fatal("original block lost its exercise")
	}

	if _, err := f.store.SetBlockClients(ctx, f.trainer, f.programID, f.weekID, dup.ID, nil); err != nil {
		t.Fatalf("SetBlockClients clear copy: %v", err)
	}
	after := f.detail(t)
	requireClients(t, findBlock(t, after, source.ID), f.client)
	requireClients(t, findBlock(t, after, dup.ID))
}

func TestDuplicateBlockExercise_notFound(t *testing.T) {
	f := newSettingsFixture(t, "dup-notfound")
	ctx := context.Background()
	group := f.addGroup(t)
	other := f.addSingle(t, "3", "10")

	cases := []struct {
		name    string
		weekID  uuid.UUID
		blockID uuid.UUID
		itemID  uuid.UUID
	}{
		{"unknown block", f.weekID, uuid.New(), group.Exercises[0].ID},
		{"unknown item", f.weekID, group.ID, uuid.New()},
		{"item of another block", f.weekID, group.ID, other.Exercises[0].ID},
		{"block of another week", uuid.New(), group.ID, group.Exercises[0].ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.store.DuplicateBlockExercise(ctx, f.trainer, f.programID, tc.weekID, tc.blockID, tc.itemID)
			if !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("error = %v, want pgx.ErrNoRows", err)
			}
		})
	}
	if got := len(f.block(t, group.ID).Exercises); got != 2 {
		t.Fatalf("group exercises = %d, want 2 (nothing written)", got)
	}
}

func TestCopyDayBlock_toAnotherWeekCopiesEverything(t *testing.T) {
	f := newPublishedGroupFixture(t, "copy-week")
	ctx := context.Background()
	group := f.emomGroup(t)
	week2 := f.addWeek(t)
	target := week2.Days[0]

	if _, err := f.store.SetBlockClients(ctx, f.trainer, f.programID, f.weekID, group.ID, []uuid.UUID{f.client}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}
	// Give the target day one block so "appended at the end" is observable.
	d, err := f.store.CopyDayBlock(ctx, f.trainer, f.programID, f.weekID, f.blocks[2].ID, target.ID)
	if err != nil {
		t.Fatalf("CopyDayBlock single: %v", err)
	}
	filler := mustDay(t, d, target.ID).Blocks[0]

	d, err = f.store.CopyDayBlock(ctx, f.trainer, f.programID, f.weekID, group.ID, target.ID)
	if err != nil {
		t.Fatalf("CopyDayBlock: %v", err)
	}
	day := mustDay(t, d, target.ID)
	if len(day.Blocks) != 2 {
		t.Fatalf("target day blocks = %d, want 2", len(day.Blocks))
	}
	requireBlockSortOrders(t, day.Blocks)
	if day.Blocks[0].ID != filler.ID {
		t.Fatal("copy was not appended after the existing block")
	}
	cp := day.Blocks[1]
	original := findBlock(t, d, group.ID)
	if cp.ID == original.ID || cp.BlockKey == original.BlockKey {
		t.Fatalf("copy must be a new block with a new block_key, got id=%s key=%s", cp.ID, cp.BlockKey)
	}
	if cp.BlockType != program.BlockTypeEMOM || cp.Instruction != "every minute" {
		t.Fatalf("copy = %s %q, want emom %q", cp.BlockType, cp.Instruction, "every minute")
	}
	assertSettings(t, cp, ip(5), nil, nil, nil, nil)
	if len(cp.Exercises) != len(original.Exercises) {
		t.Fatalf("copy exercises = %d, want %d", len(cp.Exercises), len(original.Exercises))
	}
	requireExerciseSortOrders(t, cp.Exercises)
	for i := range original.Exercises {
		requireCopiedExercise(t, cp.Exercises[i], original.Exercises[i])
	}
	requireClients(t, cp, f.client)

	// The copy's rules are independent rows: clearing them leaves the original alone.
	if _, err := f.store.SetBlockClients(ctx, f.trainer, f.programID, week2.ID, cp.ID, nil); err != nil {
		t.Fatalf("SetBlockClients clear copy: %v", err)
	}
	after := f.detail(t)
	requireClients(t, findBlock(t, after, group.ID), f.client)
	requireClients(t, findBlock(t, after, cp.ID))

	// And the other direction: the original's rules do not follow the copy.
	if _, err := f.store.SetBlockClients(ctx, f.trainer, f.programID, week2.ID, cp.ID, []uuid.UUID{f.client}); err != nil {
		t.Fatalf("SetBlockClients restrict copy: %v", err)
	}
	if _, err := f.store.SetBlockClients(ctx, f.trainer, f.programID, f.weekID, group.ID, nil); err != nil {
		t.Fatalf("SetBlockClients clear original: %v", err)
	}
	requireClients(t, findBlock(t, f.detail(t), cp.ID), f.client)
}

func mustDay(t *testing.T, d program.Detail, dayID uuid.UUID) program.Day {
	t.Helper()
	for _, w := range d.Weeks {
		if day, ok := findDay(w, dayID); ok {
			return day
		}
	}
	t.Fatalf("day %s not found", dayID)
	return program.Day{}
}

func TestCopyDayBlock_toSameDayAppendsAtEnd(t *testing.T) {
	f := newPublishedGroupFixture(t, "copy-same-day")
	ctx := context.Background()
	group := f.emomGroup(t)

	d, err := f.store.CopyDayBlock(ctx, f.trainer, f.programID, f.weekID, group.ID, f.dayID)
	if err != nil {
		t.Fatalf("CopyDayBlock: %v", err)
	}
	day := mustDay(t, d, f.dayID)
	if len(day.Blocks) != 4 {
		t.Fatalf("blocks in day = %d, want 4 (group, 2 singles, copy)", len(day.Blocks))
	}
	requireBlockSortOrders(t, day.Blocks)
	cp := day.Blocks[3]
	if cp.ID == group.ID || cp.BlockType != program.BlockTypeEMOM || len(cp.Exercises) != 2 {
		t.Fatalf("last block = %s %s with %d exercises, want the emom copy", cp.ID, cp.BlockType, len(cp.Exercises))
	}
	assertSettings(t, cp, ip(5), nil, nil, nil, nil)
}

func TestCopyDayBlock_singleBlock(t *testing.T) {
	f := newPublishedGroupFixture(t, "copy-single")
	ctx := context.Background()

	d, err := f.store.CopyDayBlock(ctx, f.trainer, f.programID, f.weekID, f.blocks[0].ID, f.dayID)
	if err != nil {
		t.Fatalf("CopyDayBlock: %v", err)
	}
	day := mustDay(t, d, f.dayID)
	if len(day.Blocks) != 5 {
		t.Fatalf("blocks in day = %d, want 5", len(day.Blocks))
	}
	cp := day.Blocks[4]
	original := findBlock(t, d, f.blocks[0].ID)
	if cp.BlockType != program.BlockTypeSingle || len(cp.Exercises) != 1 {
		t.Fatalf("copy = %s with %d exercises, want a single with one", cp.BlockType, len(cp.Exercises))
	}
	requireCopiedExercise(t, cp.Exercises[0], original.Exercises[0])
}

func TestCopyDayBlock_notFound(t *testing.T) {
	f := newPublishedGroupFixture(t, "copy-notfound")
	ctx := context.Background()
	group := f.emomGroup(t)

	_, _, otherBlocks, _ := seedDayWithBlocks(t, f.pool, "copy-notfound-other", 1)
	otherDay := dayIDOfBlock(t, f.pool, otherBlocks[0].ID)

	cases := []struct {
		name     string
		weekID   uuid.UUID
		blockID  uuid.UUID
		targetID uuid.UUID
	}{
		{"unknown block", f.weekID, uuid.New(), f.dayID},
		{"block of another week", uuid.New(), group.ID, f.dayID},
		{"missing target day", f.weekID, group.ID, uuid.New()},
		{"day of another program", f.weekID, group.ID, otherDay},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.store.CopyDayBlock(ctx, f.trainer, f.programID, tc.weekID, tc.blockID, tc.targetID)
			if !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("error = %v, want pgx.ErrNoRows", err)
			}
		})
	}
	if got := len(mustDay(t, f.detail(t), f.dayID).Blocks); got != 3 {
		t.Fatalf("blocks in day = %d, want 3 (nothing written)", got)
	}
}

func TestCopyDayBlock_limitedBlockToDayWithoutSharedBlock(t *testing.T) {
	f := newPublishedGroupFixture(t, "copy-limited")
	ctx := context.Background()
	week2 := f.addWeek(t)
	target := week2.Days[0]

	if _, err := f.store.SetBlockClients(ctx, f.trainer, f.programID, f.weekID, f.blocks[0].ID, []uuid.UUID{f.client}); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}
	d, err := f.store.CopyDayBlock(ctx, f.trainer, f.programID, f.weekID, f.blocks[0].ID, target.ID)
	if err != nil {
		t.Fatalf("CopyDayBlock into a day without a shared block: %v", err)
	}
	day := mustDay(t, d, target.ID)
	if len(day.Blocks) != 1 {
		t.Fatalf("target day blocks = %d, want 1", len(day.Blocks))
	}
	requireClients(t, day.Blocks[0], f.client)
}
