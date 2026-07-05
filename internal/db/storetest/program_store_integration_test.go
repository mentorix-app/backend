//go:build integration

package storetest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
)

func TestProgramStore_CreateDraftAndPublish(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-store@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, exercise.UpsertInput{
		Name:        "Bench Press",
		NameRu:      "Жим",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	progStore := program.NewStore(pool)
	svc := program.NewService(pool)

	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft() error = %v", err)
	}
	if draft.Status != program.StatusDraft {
		t.Errorf("status = %q, want draft", draft.Status)
	}
	if len(draft.Weeks) != 1 {
		t.Fatalf("weeks len = %d, want 1", len(draft.Weeks))
	}
	if len(draft.Weeks[0].Days) != program.DefaultWeekDays {
		t.Fatalf("days len = %d, want %d", len(draft.Weeks[0].Days), program.DefaultWeekDays)
	}

	_, err = svc.Publish(ctx, trainerID, draft.ID)
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("Publish() on empty draft error = %v, want ErrValidation", err)
	}

	name := "Strength Block"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	updated, err := progStore.Update(ctx, draft.ID, trainerID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != name {
		t.Errorf("name = %q, want %q", updated.Name, name)
	}

	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := 3, 10
	withExercise, err := createSingleBlockStore(ctx, progStore, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	if dayBlockCount(withExercise.Weeks[0].Days[0]) != 1 {
		t.Fatalf("day blocks len = %d, want 1", dayBlockCount(withExercise.Weeks[0].Days[0]))
	}
	if dayExerciseCount(withExercise.Weeks[0].Days[0]) != 1 {
		t.Fatalf("day exercises len = %d, want 1", dayExerciseCount(withExercise.Weeks[0].Days[0]))
	}

	published, err := svc.Publish(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if published.Status != program.StatusPublished {
		t.Errorf("status = %q, want published", published.Status)
	}

	archived, err := svc.Archive(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if archived.Status != program.StatusArchived {
		t.Errorf("status = %q, want archived", archived.Status)
	}
}

func TestProgramStore_ListFiltersByTrainer(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerA, err := authStore.RegisterTrainerEmailPassword(ctx, "trainer-a@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer A: %v", err)
	}
	trainerB, err := authStore.RegisterTrainerEmailPassword(ctx, "trainer-b@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer B: %v", err)
	}

	progStore := program.NewStore(pool)
	svc := program.NewService(pool)

	if _, err := progStore.CreateDraft(ctx, trainerA); err != nil {
		t.Fatalf("create draft A: %v", err)
	}
	if _, err := progStore.CreateDraft(ctx, trainerB); err != nil {
		t.Fatalf("create draft B: %v", err)
	}

	params, err := program.ParseListParams("1", "20", "created_at", "desc", "", "", "", "")
	if err != nil {
		t.Fatalf("ParseListParams() error = %v", err)
	}

	listA, err := svc.List(ctx, trainerA, params)
	if err != nil {
		t.Fatalf("List() trainer A error = %v", err)
	}
	if listA.Pagination.Total != 1 {
		t.Errorf("trainer A total = %d, want 1", listA.Pagination.Total)
	}
	if len(listA.Items) == 1 && listA.Items[0].CreatedBy != trainerA {
		t.Errorf("created_by = %v, want %v", listA.Items[0].CreatedBy, trainerA)
	}
}

func TestProgramStore_dayAndExerciseLifecycle(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-lifecycle@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, exercise.UpsertInput{
		Name:        "Row",
		NameRu:      "Тяга",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	weekID := draft.Weeks[0].ID
	_, err = progStore.AddDay(ctx, draft.ID, weekID)
	if !errors.Is(err, program.ErrMaxDaysPerWeek) {
		t.Fatalf("AddDay at max days error = %v, want ErrMaxDaysPerWeek", err)
	}

	dayIdx := program.DefaultWeekDays - 1
	dayID := draft.Weeks[0].Days[dayIdx].ID

	sets, reps := 4, 8
	withExercise, err := createSingleBlockStore(ctx, progStore, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	var itemID uuid.UUID
	var blockID uuid.UUID
	for _, day := range withExercise.Weeks[0].Days {
		if day.ID == dayID {
			ex, ok := firstDayExercise(day)
			if ok {
				itemID = ex.ID
			}
			b, ok := firstDayBlock(day)
			if ok {
				blockID = b.ID
			}
			break
		}
	}
	if itemID == uuid.Nil || blockID == uuid.Nil {
		t.Fatal("expected exercise and block on day")
	}

	newSets := 5
	updatedExercise, err := progStore.UpdateBlockExercise(ctx, trainerID, draft.ID, weekID, blockID, itemID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &newSets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("UpdateDayExercise: %v", err)
	}
	var updatedSets int
	for _, day := range updatedExercise.Weeks[0].Days {
		if day.ID == dayID {
			ex, ok := firstDayExercise(day)
			if ok {
				updatedSets = *ex.Sets
			}
			break
		}
	}
	if updatedSets != newSets {
		t.Fatalf("sets = %d, want %d", updatedSets, newSets)
	}

	withoutExercise, err := progStore.DeleteBlockExercise(ctx, draft.ID, weekID, blockID, itemID)
	if err != nil {
		t.Fatalf("DeleteBlockExercise: %v", err)
	}
	var exercisesLeft int
	for _, day := range withoutExercise.Weeks[0].Days {
		if day.ID == dayID {
			exercisesLeft = dayExerciseCount(day)
			break
		}
	}
	if exercisesLeft != 0 {
		t.Fatalf("expected day exercise removed")
	}

	oneLessDay, err := progStore.DeleteDay(ctx, draft.ID, weekID, dayID)
	if err != nil {
		t.Fatalf("DeleteDay: %v", err)
	}
	if len(oneLessDay.Weeks[0].Days) != program.DefaultWeekDays-1 {
		t.Fatalf("days = %d, want %d", len(oneLessDay.Weeks[0].Days), program.DefaultWeekDays-1)
	}

	withAddedDay, err := progStore.AddDay(ctx, draft.ID, weekID)
	if err != nil {
		t.Fatalf("AddDay after delete: %v", err)
	}
	if len(withAddedDay.Weeks[0].Days) != program.DefaultWeekDays {
		t.Fatalf("days = %d, want %d", len(withAddedDay.Weeks[0].Days), program.DefaultWeekDays)
	}

	if err := progStore.SoftDelete(ctx, draft.ID, trainerID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
}

func TestProgramStore_WeeksAndReorder(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-weeks@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, exercise.UpsertInput{
		Name:        "Press",
		NameRu:      "Жим",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if len(draft.Weeks) != 1 || len(draft.Weeks[0].Days) != program.DefaultWeekDays {
		t.Fatalf("initial weeks/days = %d/%d", len(draft.Weeks), len(draft.Weeks[0].Days))
	}

	week1ID := draft.Weeks[0].ID
	withWeek2, err := progStore.AddWeek(ctx, draft.ID)
	if err != nil {
		t.Fatalf("AddWeek: %v", err)
	}
	if len(withWeek2.Weeks) != 2 {
		t.Fatalf("weeks = %d, want 2", len(withWeek2.Weeks))
	}
	week2ID := withWeek2.Weeks[1].ID

	reorderedWeeks, err := progStore.ReorderWeeks(ctx, draft.ID, []uuid.UUID{week2ID, week1ID})
	if err != nil {
		t.Fatalf("ReorderWeeks: %v", err)
	}
	if reorderedWeeks.Weeks[0].ID != week2ID || reorderedWeeks.Weeks[1].ID != week1ID {
		t.Fatalf("week order not updated")
	}
	if reorderedWeeks.Weeks[0].WeekNumber != 1 || reorderedWeeks.Weeks[1].WeekNumber != 2 {
		t.Fatalf("week numbers = %d,%d", reorderedWeeks.Weeks[0].WeekNumber, reorderedWeeks.Weeks[1].WeekNumber)
	}

	_, err = progStore.ReorderWeeks(ctx, draft.ID, []uuid.UUID{week1ID})
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderWeeks partial list error = %v, want ErrInvalidReorder", err)
	}

	_, err = progStore.ReorderWeeks(ctx, draft.ID, []uuid.UUID{week1ID, week1ID})
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderWeeks duplicate ids error = %v, want ErrInvalidReorder", err)
	}

	week2 := reorderedWeeks.Weeks[0]
	dayIDs := make([]uuid.UUID, len(week2.Days))
	for i, day := range week2.Days {
		dayIDs[i] = day.ID
	}
	reversedDays := append([]uuid.UUID(nil), dayIDs...)
	for i, j := 0, len(reversedDays)-1; i < j; i, j = i+1, j-1 {
		reversedDays[i], reversedDays[j] = reversedDays[j], reversedDays[i]
	}
	reorderedDays, err := progStore.ReorderDays(ctx, draft.ID, week2ID, reversedDays)
	if err != nil {
		t.Fatalf("ReorderDays: %v", err)
	}
	if reorderedDays.Weeks[0].Days[0].ID != reversedDays[0] {
		t.Fatalf("day order not updated")
	}

	_, err = progStore.ReorderDays(ctx, draft.ID, week2ID, []uuid.UUID{reversedDays[0]})
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderDays partial list error = %v, want ErrInvalidReorder", err)
	}

	duplicateDays := append([]uuid.UUID{reversedDays[0], reversedDays[0]}, reversedDays[2:]...)
	_, err = progStore.ReorderDays(ctx, draft.ID, week2ID, duplicateDays)
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderDays duplicate ids error = %v, want ErrInvalidReorder", err)
	}

	sets, reps := 3, 10
	dayA := reorderedDays.Weeks[0].Days[0].ID
	dayB := reorderedDays.Weeks[0].Days[1].ID
	withExA, err := createSingleBlockStore(ctx, progStore, trainerID, draft.ID, week2ID, dayA, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise dayA: %v", err)
	}
	var blockA uuid.UUID
	for _, day := range withExA.Weeks[0].Days {
		if day.ID == dayA {
			b, ok := firstDayBlock(day)
			if ok {
				blockA = b.ID
			}
			break
		}
	}
	if blockA == uuid.Nil {
		t.Fatal("expected block on dayA")
	}

	withExB, err := createSingleBlockStore(ctx, progStore, trainerID, draft.ID, week2ID, dayB, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise dayB: %v", err)
	}
	var blockB uuid.UUID
	for _, day := range withExB.Weeks[0].Days {
		if day.ID == dayB {
			b, ok := firstDayBlock(day)
			if ok {
				blockB = b.ID
			}
			break
		}
	}
	if blockB == uuid.Nil {
		t.Fatal("expected block on dayB")
	}

	_, err = progStore.ReorderBlockExercises(ctx, trainerID, draft.ID, week2ID, uuid.New(), nil)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ReorderBlockExercises unknown block error = %v, want ErrNoRows", err)
	}

	_, err = progStore.ReorderBlockExercises(ctx, trainerID, draft.ID, week2ID, blockB, []uuid.UUID{uuid.New()})
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderBlockExercises unknown item error = %v, want ErrInvalidReorder", err)
	}

	moved, err := progStore.MoveDayBlock(ctx, trainerID, draft.ID, week2ID, blockA, dayB, 0)
	if err != nil {
		t.Fatalf("MoveDayBlock: %v", err)
	}
	dayBAfterMove, ok := findDay(moved.Weeks[0], dayB)
	if !ok {
		t.Fatal("dayB not found")
	}
	if dayBlockCount(dayBAfterMove) != 2 {
		t.Fatalf("dayB blocks = %d, want 2", dayBlockCount(dayBAfterMove))
	}

	blockIDs := make([]uuid.UUID, 0, 2)
	for _, b := range dayBAfterMove.Blocks {
		blockIDs = append(blockIDs, b.ID)
	}
	merged, err := progStore.MergeDayBlocks(ctx, trainerID, draft.ID, week2ID, dayB, blockIDs)
	if err != nil {
		t.Fatalf("MergeDayBlocks: %v", err)
	}
	dayBMerged, ok := findDay(merged.Weeks[0], dayB)
	if !ok {
		t.Fatal("dayB not found after merge")
	}
	var groupBlock program.DayBlock
	for _, b := range dayBMerged.Blocks {
		if b.BlockType != program.BlockTypeSingle {
			groupBlock = b
			break
		}
	}
	if len(groupBlock.Exercises) < 2 {
		t.Fatalf("group exercises = %d, want at least 2", len(groupBlock.Exercises))
	}
	_, err = progStore.ReorderBlockExercises(ctx, trainerID, draft.ID, week2ID, groupBlock.ID, []uuid.UUID{groupBlock.Exercises[0].ID})
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderBlockExercises partial list error = %v, want ErrInvalidReorder", err)
	}
	reorderedExerciseIDs := make([]uuid.UUID, len(groupBlock.Exercises))
	for i, ex := range groupBlock.Exercises {
		reorderedExerciseIDs[len(groupBlock.Exercises)-1-i] = ex.ID
	}
	if _, err = progStore.ReorderBlockExercises(ctx, trainerID, draft.ID, week2ID, groupBlock.ID, reorderedExerciseIDs); err != nil {
		t.Fatalf("ReorderBlockExercises full: %v", err)
	}

	ungroupedEarly, err := progStore.UngroupDayBlock(ctx, trainerID, draft.ID, week2ID, groupBlock.ID)
	if err != nil {
		t.Fatalf("UngroupDayBlock: %v", err)
	}
	dayBUngroupedEarly, ok := findDay(ungroupedEarly.Weeks[0], dayB)
	if !ok {
		t.Fatal("dayB not found after early ungroup")
	}
	earlyBlockIDs := make([]uuid.UUID, 0, len(dayBUngroupedEarly.Blocks))
	for _, b := range dayBUngroupedEarly.Blocks {
		if b.BlockType == program.BlockTypeSingle {
			earlyBlockIDs = append(earlyBlockIDs, b.ID)
		}
	}
	if len(earlyBlockIDs) < 2 {
		t.Fatalf("single blocks after ungroup = %d, want at least 2", len(earlyBlockIDs))
	}
	merged, err = progStore.MergeDayBlocks(ctx, trainerID, draft.ID, week2ID, dayB, earlyBlockIDs)
	if err != nil {
		t.Fatalf("MergeDayBlocks after ungroup: %v", err)
	}
	dayBMerged, ok = findDay(merged.Weeks[0], dayB)
	if !ok {
		t.Fatal("dayB not found after re-merge")
	}
	groupBlock = program.DayBlock{}
	for _, b := range dayBMerged.Blocks {
		if b.BlockType != program.BlockTypeSingle {
			groupBlock = b
			break
		}
	}
	if groupBlock.ID == uuid.Nil {
		t.Fatal("expected group block after re-merge")
	}

	emom := program.BlockTypeEMOM
	instr := "60s"
	patched, err := progStore.PatchDayBlock(ctx, trainerID, draft.ID, week2ID, groupBlock.ID, program.BlockPatchInput{
		BlockType:   &emom,
		Instruction: &instr,
	})
	if err != nil {
		t.Fatalf("PatchDayBlock: %v", err)
	}
	dayBPatched, ok := findDay(patched.Weeks[0], dayB)
	if !ok {
		t.Fatal("dayB not found after patch")
	}
	var patchedGroup program.DayBlock
	for _, b := range dayBPatched.Blocks {
		if b.BlockType == program.BlockTypeEMOM {
			patchedGroup = b
			break
		}
	}
	if patchedGroup.ID == uuid.Nil {
		t.Fatal("expected EMOM group block after patch")
	}

	withAdded, err := progStore.AddBlockExercise(ctx, trainerID, draft.ID, week2ID, patchedGroup.ID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddBlockExercise: %v", err)
	}
	dayBWithAdded, ok := findDay(withAdded.Weeks[0], dayB)
	if !ok {
		t.Fatal("dayB not found after add exercise")
	}
	var groupAfterAdd program.DayBlock
	for _, b := range dayBWithAdded.Blocks {
		if b.BlockType != program.BlockTypeSingle {
			groupAfterAdd = b
			break
		}
	}
	if len(groupAfterAdd.Exercises) < 3 {
		t.Fatalf("group exercises = %d, want at least 3", len(groupAfterAdd.Exercises))
	}

	dayBlockIDs := make([]uuid.UUID, 0, len(dayBWithAdded.Blocks))
	for _, b := range dayBWithAdded.Blocks {
		dayBlockIDs = append(dayBlockIDs, b.ID)
	}
	reorderedBlocks, err := progStore.ReorderDayBlocks(ctx, trainerID, draft.ID, week2ID, dayB, dayBlockIDs)
	if err != nil {
		t.Fatalf("ReorderDayBlocks: %v", err)
	}
	dayBReordered, ok := findDay(reorderedBlocks.Weeks[0], dayB)
	if !ok {
		t.Fatal("dayB not found after reorder blocks")
	}
	if len(dayBReordered.Blocks) != len(dayBlockIDs) {
		t.Fatalf("blocks = %d, want %d", len(dayBReordered.Blocks), len(dayBlockIDs))
	}
	if len(dayBlockIDs) > 1 {
		_, err = progStore.ReorderDayBlocks(ctx, trainerID, draft.ID, week2ID, dayB, dayBlockIDs[:1])
		if !errors.Is(err, program.ErrInvalidReorder) {
			t.Fatalf("ReorderDayBlocks partial list error = %v, want ErrInvalidReorder", err)
		}
	}

	extractItemID := groupAfterAdd.Exercises[0].ID
	extracted, err := progStore.ExtractBlockExercise(ctx, trainerID, draft.ID, week2ID, groupAfterAdd.ID, extractItemID, 0)
	if err != nil {
		t.Fatalf("ExtractBlockExercise: %v", err)
	}
	dayBExtracted, ok := findDay(extracted.Weeks[0], dayB)
	if !ok {
		t.Fatal("dayB not found after extract")
	}
	if dayBlockCount(dayBExtracted) < 2 {
		t.Fatalf("blocks after extract = %d, want at least 2", dayBlockCount(dayBExtracted))
	}

	var remainingGroup program.DayBlock
	var singleBlock program.DayBlock
	for _, b := range dayBExtracted.Blocks {
		if b.BlockType != program.BlockTypeSingle {
			remainingGroup = b
		} else if singleBlock.ID == uuid.Nil {
			singleBlock = b
		}
	}
	if remainingGroup.ID == uuid.Nil || singleBlock.ID == uuid.Nil {
		t.Fatal("expected group and single block after extract")
	}

	if len(singleBlock.Exercises) > 0 {
		_, err = progStore.MoveExerciseToBlock(ctx, trainerID, draft.ID, week2ID, singleBlock.ID, singleBlock.Exercises[0].ID, remainingGroup.ID)
		if err != nil {
			t.Fatalf("MoveExerciseToBlock: %v", err)
		}
	}

	_, err = progStore.DeleteDayBlock(ctx, draft.ID, week2ID, remainingGroup.ID)
	if err != nil {
		t.Fatalf("DeleteDayBlock: %v", err)
	}

	afterDeleteWeek, err := progStore.DeleteWeek(ctx, draft.ID, week2ID)
	if err != nil {
		t.Fatalf("DeleteWeek week2: %v", err)
	}
	if len(afterDeleteWeek.Weeks) != 1 {
		t.Fatalf("weeks = %d, want 1", len(afterDeleteWeek.Weeks))
	}

	_, err = progStore.DeleteWeek(ctx, draft.ID, afterDeleteWeek.Weeks[0].ID)
	if !errors.Is(err, program.ErrLastWeek) {
		t.Fatalf("DeleteWeek last week error = %v, want ErrLastWeek", err)
	}
}

func TestProgramStore_DeleteDay_lastDay(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-last-day@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	weekID := draft.Weeks[0].ID
	detail := draft
	for len(detail.Weeks[0].Days) > 1 {
		dayID := detail.Weeks[0].Days[len(detail.Weeks[0].Days)-1].ID
		detail, err = progStore.DeleteDay(ctx, draft.ID, weekID, dayID)
		if err != nil {
			t.Fatalf("DeleteDay: %v", err)
		}
	}
	lastDayID := detail.Weeks[0].Days[0].ID
	_, err = progStore.DeleteDay(ctx, draft.ID, weekID, lastDayID)
	if !errors.Is(err, program.ErrLastDay) {
		t.Fatalf("DeleteDay last day error = %v, want ErrLastDay", err)
	}
}

func TestProgramStore_reorderUnknownWeek(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-unknown-week@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	dayID := draft.Weeks[0].Days[0].ID

	_, err = progStore.ReorderDays(ctx, draft.ID, uuid.New(), []uuid.UUID{dayID})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ReorderDays unknown week error = %v, want ErrNoRows", err)
	}

	_, err = progStore.AddDay(ctx, draft.ID, uuid.New())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("AddDay unknown week error = %v, want ErrNoRows", err)
	}
}

func TestProgramStore_ListWithFilters(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-list@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	name := "My Program"
	if _, err := progStore.Update(ctx, draft.ID, trainerID, program.UpdateInput{Name: &name}); err != nil {
		t.Fatalf("update name: %v", err)
	}

	params, err := program.ParseListParams("1", "20", "created_at", "desc", "program", "draft", "", "")
	if err != nil {
		t.Fatalf("ParseListParams: %v", err)
	}
	params.CreatedBy = &trainerID
	result, err := progStore.List(ctx, params)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Pagination.Total < 1 {
		t.Fatalf("total = %d, want at least 1", result.Pagination.Total)
	}

	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	paramsCat, err := program.ParseListParams("1", "20", "created_at", "desc", "", "draft", string(category), string(difficulty))
	if err != nil {
		t.Fatalf("ParseListParams category: %v", err)
	}
	paramsCat.CreatedBy = &trainerID
	if _, err := progStore.Update(ctx, draft.ID, trainerID, program.UpdateInput{
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("update filters: %v", err)
	}
	filtered, err := progStore.List(ctx, paramsCat)
	if err != nil {
		t.Fatalf("List with filters: %v", err)
	}
	if filtered.Pagination.Total < 1 {
		t.Fatalf("filtered total = %d", filtered.Pagination.Total)
	}
}

func TestProgramStore_GetNotFound(t *testing.T) {
	pool := NewPool(t)
	store := program.NewStore(pool)
	_, err := store.GetProgramRow(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestAuthUserIsAdmin_integration(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := authStore.RegisterTrainerEmailPassword(ctx, "admin-check@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	q := sqlc.New(pool)
	isAdmin, err := auth.UserIsAdmin(ctx, q, userID)
	if err != nil {
		t.Fatalf("UserIsAdmin() error = %v", err)
	}
	if isAdmin {
		t.Fatal("trainer should not be admin yet")
	}
	if err := authStore.GrantRole(ctx, userID, auth.RoleAdmin); err != nil {
		t.Fatalf("GrantRole: %v", err)
	}
	isAdmin, err = auth.UserIsAdmin(ctx, q, userID)
	if err != nil || !isAdmin {
		t.Fatalf("UserIsAdmin() = %v, %v, want true", isAdmin, err)
	}
}

func TestProgramService_adminMutatesOtherUsersProgram(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	ownerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-owner@test.com", pwHash, "Owner")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	adminID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-admin@test.com", pwHash, "Admin")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if err := authStore.GrantRole(ctx, adminID, auth.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, ownerID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	name := "Edited by admin"
	if _, err := svc.Update(ctx, adminID, draft.ID, program.UpdateInput{Name: &name}); err != nil {
		t.Fatalf("Update() by admin error = %v", err)
	}
	if err := svc.Delete(ctx, adminID, draft.ID); err != nil {
		t.Fatalf("Delete() by admin error = %v", err)
	}
}

func TestProgramStore_blockValidationErrors(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "block-val@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, exercise.UpsertInput{
		Name:        "Curl",
		NameRu:      "Сгибание",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupArms,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := 3, 10
	withBlock, err := createSingleBlockStore(ctx, progStore, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("create block: %v", err)
	}
	var block program.DayBlock
	for _, d := range withBlock.Weeks[0].Days {
		if d.ID == dayID {
			b, ok := firstDayBlock(d)
			if ok {
				block = b
			}
			break
		}
	}
	if block.ID == uuid.Nil {
		t.Fatal("expected block")
	}
	itemID := block.Exercises[0].ID

	_, err = progStore.MergeDayBlocks(ctx, trainerID, draft.ID, weekID, dayID, []uuid.UUID{block.ID})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("MergeDayBlocks one block error = %v, want ErrValidation", err)
	}

	_, err = progStore.DeleteDayBlock(ctx, draft.ID, weekID, block.ID)
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("DeleteDayBlock single error = %v, want ErrValidation", err)
	}

	_, err = progStore.ExtractBlockExercise(ctx, trainerID, draft.ID, weekID, block.ID, itemID, 0)
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("ExtractBlockExercise single error = %v, want ErrValidation", err)
	}

	_, err = progStore.MoveExerciseToBlock(ctx, trainerID, draft.ID, weekID, block.ID, itemID, block.ID)
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("MoveExerciseToBlock into single error = %v, want ErrValidation", err)
	}

	_, err = progStore.AddBlockExercise(ctx, trainerID, draft.ID, weekID, block.ID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("AddBlockExercise to single block error = %v, want ErrValidation", err)
	}
}

func TestProgramStore_blockNotFound(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "block-nf@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	unknownBlock := uuid.New()
	unknownItem := uuid.New()
	emom := program.BlockTypeEMOM

	_, err = progStore.PatchDayBlock(ctx, trainerID, draft.ID, weekID, unknownBlock, program.BlockPatchInput{BlockType: &emom})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("PatchDayBlock: %v", err)
	}
	_, err = progStore.UngroupDayBlock(ctx, trainerID, draft.ID, weekID, unknownBlock)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("UngroupDayBlock: %v", err)
	}
	_, err = progStore.MoveDayBlock(ctx, trainerID, draft.ID, weekID, unknownBlock, dayID, 0)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("MoveDayBlock: %v", err)
	}
	_, err = progStore.ExtractBlockExercise(ctx, trainerID, draft.ID, weekID, unknownBlock, unknownItem, 0)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ExtractBlockExercise: %v", err)
	}
	_, err = progStore.MoveExerciseToBlock(ctx, trainerID, draft.ID, weekID, unknownBlock, unknownItem, unknownBlock)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("MoveExerciseToBlock: %v", err)
	}
	_, err = progStore.DeleteDayBlock(ctx, draft.ID, weekID, unknownBlock)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("DeleteDayBlock: %v", err)
	}
}

func TestProgramStore_groupBlockExerciseCRUD(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "group-crud@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	ex1, err := exStore.Create(ctx, trainerID, exercise.UpsertInput{
		Name: "A", NameRu: "А", Type: exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest, Difficulty: exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create ex1: %v", err)
	}
	ex2, err := exStore.Create(ctx, trainerID, exercise.UpsertInput{
		Name: "B", NameRu: "Б", Type: exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack, Difficulty: exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create ex2: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := 3, 10

	withA, err := createSingleBlockStore(ctx, progStore, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: ex1.ID, Sets: &sets, Reps: &reps,
	})
	if err != nil {
		t.Fatalf("block A: %v", err)
	}
	withBoth, err := createSingleBlockStore(ctx, progStore, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: ex2.ID, Sets: &sets, Reps: &reps,
	})
	if err != nil {
		t.Fatalf("block B: %v", err)
	}

	dayA, ok := findDay(withBoth.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found")
	}
	_ = withA
	blockIDs := make([]uuid.UUID, 0, len(dayA.Blocks))
	for _, b := range dayA.Blocks {
		blockIDs = append(blockIDs, b.ID)
	}
	if len(blockIDs) < 2 {
		t.Fatalf("blocks = %d, want 2", len(blockIDs))
	}

	merged, err := progStore.MergeDayBlocks(ctx, trainerID, draft.ID, weekID, dayID, blockIDs)
	if err != nil {
		t.Fatalf("MergeDayBlocks: %v", err)
	}
	dayMerged, ok := findDay(merged.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found after merge")
	}
	var groupBlock program.DayBlock
	for _, b := range dayMerged.Blocks {
		if b.BlockType != program.BlockTypeSingle {
			groupBlock = b
			break
		}
	}
	if groupBlock.ID == uuid.Nil {
		t.Fatal("expected group block")
	}

	withAdded, err := progStore.AddBlockExercise(ctx, trainerID, draft.ID, weekID, groupBlock.ID, program.DayExerciseInput{
		ExerciseID: ex1.ID, Sets: &sets, Reps: &reps,
	})
	if err != nil {
		t.Fatalf("AddBlockExercise: %v", err)
	}
	dayAdded, ok := findDay(withAdded.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found after add")
	}
	var groupAfter program.DayBlock
	for _, b := range dayAdded.Blocks {
		if b.ID == groupBlock.ID {
			groupAfter = b
			break
		}
	}
	if len(groupAfter.Exercises) < 3 {
		t.Fatalf("exercises = %d, want at least 3", len(groupAfter.Exercises))
	}

	_, err = progStore.AddBlockExercise(ctx, trainerID, draft.ID, weekID, groupBlock.ID, program.DayExerciseInput{
		ExerciseID: uuid.New(), Sets: &sets, Reps: &reps,
	})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("AddBlockExercise unknown exercise: %v, want ErrValidation", err)
	}

	itemID := groupAfter.Exercises[0].ID
	_, err = progStore.UpdateBlockExercise(ctx, trainerID, draft.ID, weekID, groupBlock.ID, itemID, program.DayExerciseInput{
		ExerciseID: uuid.New(), Sets: &sets, Reps: &reps,
	})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("UpdateBlockExercise unknown exercise: %v, want ErrValidation", err)
	}

	_, err = progStore.UpdateBlockExercise(ctx, trainerID, draft.ID, weekID, groupBlock.ID, uuid.New(), program.DayExerciseInput{
		ExerciseID: ex1.ID, Sets: &sets, Reps: &reps,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("UpdateBlockExercise unknown item: %v, want ErrNoRows", err)
	}

	_, err = progStore.DeleteBlockExercise(ctx, draft.ID, weekID, groupBlock.ID, uuid.New())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("DeleteBlockExercise unknown item: %v, want ErrNoRows", err)
	}

	afterDelete, err := progStore.DeleteBlockExercise(ctx, draft.ID, weekID, groupBlock.ID, itemID)
	if err != nil {
		t.Fatalf("DeleteBlockExercise: %v", err)
	}
	dayAfterDelete, ok := findDay(afterDelete.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found after delete exercise")
	}
	var exercisesLeft int
	for _, b := range dayAfterDelete.Blocks {
		if b.ID == groupBlock.ID {
			exercisesLeft = len(b.Exercises)
			break
		}
	}
	if exercisesLeft != len(groupAfter.Exercises)-1 {
		t.Fatalf("exercises left = %d, want %d", exercisesLeft, len(groupAfter.Exercises)-1)
	}

	_, err = progStore.DeleteBlockExercise(ctx, draft.ID, weekID, uuid.New(), itemID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("DeleteBlockExercise unknown block: %v, want ErrNoRows", err)
	}
}

func TestProgramStore_twoGroupsMoveAndMergeInstructions(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "two-groups@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	exercises := make([]exercise.Exercise, 4)
	for i := range exercises {
		exercises[i], err = exStore.Create(ctx, trainerID, exercise.UpsertInput{
			Name:        "Ex",
			NameRu:      "Упр",
			Type:        exercise.ExerciseTypeStrength,
			MuscleGroup: exercise.MuscleGroupChest,
			Difficulty:  exercise.DifficultyBeginner,
		})
		if err != nil {
			t.Fatalf("create exercise %d: %v", i, err)
		}
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := 3, 10

	detail := draft
	for _, ex := range exercises {
		detail, err = createSingleBlockStore(ctx, progStore, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
			ExerciseID: ex.ID, Sets: &sets, Reps: &reps,
		})
		if err != nil {
			t.Fatalf("create single block: %v", err)
		}
	}

	day, ok := findDay(detail.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found")
	}
	if len(day.Blocks) < 4 {
		t.Fatalf("blocks = %d, want at least 4", len(day.Blocks))
	}
	blockIDs := make([]uuid.UUID, len(day.Blocks))
	for i, b := range day.Blocks {
		blockIDs[i] = b.ID
	}

	mergedA, err := progStore.MergeDayBlocks(ctx, trainerID, draft.ID, weekID, dayID, blockIDs[:2])
	if err != nil {
		t.Fatalf("MergeDayBlocks A: %v", err)
	}
	dayA, ok := findDay(mergedA.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found after merge A")
	}
	var groupA program.DayBlock
	for _, b := range dayA.Blocks {
		if b.BlockType != program.BlockTypeSingle {
			groupA = b
			break
		}
	}
	if groupA.ID == uuid.Nil {
		t.Fatal("expected group A")
	}

	emom := program.BlockTypeEMOM
	instrA := "EMOM 60s"
	if _, err = progStore.PatchDayBlock(ctx, trainerID, draft.ID, weekID, groupA.ID, program.BlockPatchInput{
		BlockType: &emom, Instruction: &instrA,
	}); err != nil {
		t.Fatalf("PatchDayBlock A: %v", err)
	}

	mergedB, err := progStore.MergeDayBlocks(ctx, trainerID, draft.ID, weekID, dayID, blockIDs[2:])
	if err != nil {
		t.Fatalf("MergeDayBlocks B: %v", err)
	}
	dayB, ok := findDay(mergedB.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found after merge B")
	}
	var groupB program.DayBlock
	for _, b := range dayB.Blocks {
		if b.BlockType != program.BlockTypeSingle && b.ID != groupA.ID {
			groupB = b
			break
		}
	}
	if groupB.ID == uuid.Nil {
		t.Fatal("expected group B")
	}

	amrap := program.BlockTypeAMRAP
	instrB := "20 min cap"
	patchedB, err := progStore.PatchDayBlock(ctx, trainerID, draft.ID, weekID, groupB.ID, program.BlockPatchInput{
		BlockType: &amrap, Instruction: &instrB,
	})
	if err != nil {
		t.Fatalf("PatchDayBlock B: %v", err)
	}
	dayPatched, ok := findDay(patchedB.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found after patch B")
	}
	for _, b := range dayPatched.Blocks {
		if b.ID == groupB.ID {
			groupB = b
			break
		}
	}
	if len(groupB.Exercises) == 0 {
		t.Fatal("group B has no exercises")
	}

	itemID := groupA.Exercises[0].ID
	moved, err := progStore.MoveExerciseToBlock(ctx, trainerID, draft.ID, weekID, groupA.ID, itemID, groupB.ID)
	if err != nil {
		t.Fatalf("MoveExerciseToBlock: %v", err)
	}
	dayMoved, ok := findDay(moved.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found after move")
	}
	var targetGroup program.DayBlock
	for _, b := range dayMoved.Blocks {
		if b.ID == groupB.ID {
			targetGroup = b
			break
		}
	}
	found := false
	for _, ex := range targetGroup.Exercises {
		if ex.ID == itemID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("exercise not found in target group after move")
	}

	if _, err = progStore.MoveExerciseToBlock(ctx, trainerID, draft.ID, weekID, groupB.ID, itemID, groupB.ID); err != nil {
		t.Fatalf("MoveExerciseToBlock noop: %v", err)
	}

	dayAfterMove, ok := findDay(moved.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found")
	}
	var groupIDs []uuid.UUID
	for _, b := range dayAfterMove.Blocks {
		if b.BlockType != program.BlockTypeSingle {
			groupIDs = append(groupIDs, b.ID)
		}
	}
	if len(groupIDs) < 2 {
		t.Fatalf("group blocks = %d, want at least 2", len(groupIDs))
	}

	final, err := progStore.MergeDayBlocks(ctx, trainerID, draft.ID, weekID, dayID, groupIDs)
	if err != nil {
		t.Fatalf("MergeDayBlocks final: %v", err)
	}
	dayFinal, ok := findDay(final.Weeks[0], dayID)
	if !ok {
		t.Fatal("day not found after final merge")
	}
	var mergedGroup program.DayBlock
	for _, b := range dayFinal.Blocks {
		if b.BlockType != program.BlockTypeSingle {
			mergedGroup = b
			break
		}
	}
	if mergedGroup.ID == uuid.Nil {
		t.Fatal("expected merged group")
	}
	if mergedGroup.Instruction == "" {
		t.Fatal("expected merged instruction")
	}
	if !strings.Contains(mergedGroup.Instruction, instrA) || !strings.Contains(mergedGroup.Instruction, instrB) {
		t.Fatalf("instruction = %q, want both %q and %q", mergedGroup.Instruction, instrA, instrB)
	}
}

func TestProgramStore_DeleteProgramVersion_notFound(t *testing.T) {
	pool := NewPool(t)
	store := program.NewStore(pool)
	err := store.DeleteProgramVersion(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("DeleteProgramVersion() error = %v, want ErrNotFound", err)
	}
}

func TestProgramStore_SetStatus_notFound(t *testing.T) {
	pool := NewPool(t)
	store := program.NewStore(pool)
	_, err := store.SetStatus(context.Background(), uuid.New(), uuid.New(), program.StatusArchived)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("SetStatus() error = %v, want ErrNoRows", err)
	}
}

func TestProgramStore_SoftDelete_notFound(t *testing.T) {
	pool := NewPool(t)
	store := program.NewStore(pool)
	err := store.SoftDelete(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("SoftDelete() error = %v, want ErrNoRows", err)
	}
}
