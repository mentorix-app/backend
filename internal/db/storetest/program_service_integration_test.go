//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
)

func TestProgramService_dayExerciseViaService(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-svc-day@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, nil, exercise.UpsertInput{
		Name:        "Curl",
		NameRu:      "Сгибание",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupArms,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := "3", "12"
	instruction := "controlled"
	withExercise, err := createSingleBlock(ctx, svc, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID:  catalogExercise.ID,
		Sets:        &sets,
		Reps:        &reps,
		Instruction: &instruction,
	})
	if err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	ex, ok := firstDayExercise(withExercise.Weeks[0].Days[0])
	if !ok {
		t.Fatal("expected exercise on day")
	}
	block, ok := firstDayBlock(withExercise.Weeks[0].Days[0])
	if !ok {
		t.Fatal("expected block on day")
	}
	itemID := ex.ID

	newSets := "4"
	updated, err := svc.UpdateBlockExercise(ctx, trainerID, draft.ID, weekID, block.ID, itemID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &newSets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("UpdateDayExercise() error = %v", err)
	}
	updatedEx, ok := firstDayExercise(updated.Weeks[0].Days[0])
	if !ok {
		t.Fatal("expected exercise after update")
	}
	if *updatedEx.Sets != newSets {
		t.Fatalf("sets = %q, want %q", *updatedEx.Sets, newSets)
	}

	without, err := svc.DeleteBlockExercise(ctx, trainerID, draft.ID, weekID, block.ID, itemID)
	if err != nil {
		t.Fatalf("DeleteBlockExercise() error = %v", err)
	}
	if dayExerciseCount(without.Weeks[0].Days[0]) != 0 {
		t.Fatal("expected exercise removed from day")
	}

	name := "Published Program"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	updatedMeta, err := svc.Update(ctx, trainerID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updatedMeta.Name != name {
		t.Fatalf("name = %q", updatedMeta.Name)
	}

	withExercise, err = createSingleBlock(ctx, svc, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID:  catalogExercise.ID,
		Sets:        &sets,
		Reps:        &reps,
		Instruction: &instruction,
	})
	if err != nil {
		t.Fatalf("re-add day exercise: %v", err)
	}

	published, err := svc.Publish(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if published.Status != program.StatusPublished {
		t.Fatalf("status = %q", published.Status)
	}
	if published.LatestProgramVersionID == nil {
		t.Fatal("expected latest_program_version_id after publish")
	}
	if published.HasUnpublishedChanges {
		t.Fatal("expected has_unpublished_changes false right after publish")
	}

	updatedName := "Published Program v2"
	afterEdit, err := svc.Update(ctx, trainerID, draft.ID, program.UpdateInput{Name: &updatedName})
	if err != nil {
		t.Fatalf("Update() after publish: %v", err)
	}
	if !afterEdit.HasUnpublishedChanges {
		t.Fatal("expected has_unpublished_changes true after edit")
	}

	afterPublishUpdate, err := svc.PublishUpdate(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("PublishUpdate() error = %v", err)
	}
	if afterPublishUpdate.HasUnpublishedChanges {
		t.Fatal("expected has_unpublished_changes false after publish-update")
	}
	if afterPublishUpdate.LatestProgramVersionID == nil {
		t.Fatal("expected latest_program_version_id after publish-update")
	}
	if *afterPublishUpdate.LatestProgramVersionID == *published.LatestProgramVersionID {
		t.Fatal("expected new program version id after publish-update")
	}
}

func TestProgramService_DiscardUnpublished(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-discard@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, nil, exercise.UpsertInput{
		Name:        "Row",
		NameRu:      "Тяга",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	name := "Discard Program"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, trainerID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	dayKey := draft.Weeks[0].Days[0].DayKey
	sets, reps := "3", "8"
	if _, err := createSingleBlock(ctx, svc, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("createSingleBlock: %v", err)
	}

	published, err := svc.Publish(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	publishedWeekID := published.Weeks[0].ID
	publishedDayID := published.Weeks[0].Days[0].ID
	if published.Weeks[0].Days[0].DayKey != dayKey {
		t.Fatalf("day_key after publish = %s, want %s", published.Weeks[0].Days[0].DayKey, dayKey)
	}

	dirtyName := "Should be discarded"
	afterEdit, err := svc.Update(ctx, trainerID, draft.ID, program.UpdateInput{Name: &dirtyName})
	if err != nil {
		t.Fatalf("Update dirty: %v", err)
	}
	if !afterEdit.HasUnpublishedChanges {
		t.Fatal("expected has_unpublished_changes true after edit")
	}
	if _, err := svc.AddWeek(ctx, trainerID, draft.ID); err != nil {
		t.Fatalf("AddWeek: %v", err)
	}

	restored, err := svc.DiscardUnpublished(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("DiscardUnpublished: %v", err)
	}
	if restored.HasUnpublishedChanges {
		t.Fatal("expected has_unpublished_changes false after discard")
	}
	if restored.Name != name {
		t.Fatalf("name after discard = %q, want %q", restored.Name, name)
	}
	if len(restored.Weeks) != 1 {
		t.Fatalf("weeks after discard = %d, want 1", len(restored.Weeks))
	}
	if restored.Weeks[0].ID == publishedWeekID {
		t.Fatal("expected new week id after discard")
	}
	if restored.Weeks[0].Days[0].ID == publishedDayID {
		t.Fatal("expected new day id after discard")
	}
	if restored.Weeks[0].Days[0].DayKey != dayKey {
		t.Fatalf("day_key after discard = %s, want %s", restored.Weeks[0].Days[0].DayKey, dayKey)
	}
	if restored.LatestProgramVersionID == nil || *restored.LatestProgramVersionID != *published.LatestProgramVersionID {
		t.Fatal("expected latest version unchanged after discard")
	}

	_, err = svc.DiscardUnpublished(ctx, trainerID, draft.ID)
	if !errors.Is(err, program.ErrNoUnpublishedChanges) {
		t.Fatalf("second DiscardUnpublished error = %v, want ErrNoUnpublishedChanges", err)
	}
}

func TestProgramStore_RestoreWorkingTreeFromLatestVersion_errors(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-restore-err@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	progStore := program.NewStore(pool)
	_, err = progStore.RestoreWorkingTreeFromLatestVersion(ctx, uuid.New(), trainerID)
	if !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("unknown program error = %v, want ErrNotFound", err)
	}

	draft, err := progStore.CreateDraft(ctx, trainerID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	_, err = progStore.RestoreWorkingTreeFromLatestVersion(ctx, draft.ID, trainerID)
	if !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("draft without version error = %v, want ErrNotFound", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, nil, exercise.UpsertInput{
		Name:        "Pull",
		NameRu:      "Тяга",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}
	svc := program.NewService(pool)
	name := "Restore SoftDelete"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, trainerID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	sets, reps := "2", "10"
	if _, err := createSingleBlock(ctx, svc, trainerID, draft.ID, draft.Weeks[0].ID, draft.Weeks[0].Days[0].ID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("createSingleBlock: %v", err)
	}
	if _, err := svc.Publish(ctx, trainerID, draft.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := progStore.SoftDelete(ctx, draft.ID, trainerID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	_, err = progStore.RestoreWorkingTreeFromLatestVersion(ctx, draft.ID, trainerID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("soft-deleted restore error = %v, want ErrNoRows", err)
	}
}

func TestProgramService_trainingDaysCount(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "training-days@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, nil, exercise.UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if draft.TrainingDaysCount != 0 {
		t.Fatalf("new program training_days_count = %d, want 0", draft.TrainingDaysCount)
	}

	weekID := draft.Weeks[0].ID
	day1 := draft.Weeks[0].Days[0].ID
	day2 := draft.Weeks[0].Days[1].ID
	sets, reps := "3", "10"
	if _, err := createSingleBlock(ctx, svc, trainerID, draft.ID, weekID, day1, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("day1 block: %v", err)
	}
	if _, err := createSingleBlock(ctx, svc, trainerID, draft.ID, weekID, day2, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("day2 block: %v", err)
	}

	got, err := svc.Get(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TrainingDaysCount != 2 {
		t.Fatalf("Get training_days_count = %d, want 2", got.TrainingDaysCount)
	}

	list, err := svc.List(ctx, trainerID, program.ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found *program.Program
	for i := range list.Items {
		if list.Items[i].ID == draft.ID {
			found = &list.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatal("program not in list")
	}
	if found.TrainingDaysCount != 2 {
		t.Fatalf("List training_days_count = %d, want 2", found.TrainingDaysCount)
	}
}

func TestProgramService_ListAndArchive(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-svc-list@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	svc := program.NewService(pool)
	if _, err := svc.Create(ctx, trainerID); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	params, err := program.ParseListParams("1", "20", "created_at", "desc", "", "draft", "", "")
	if err != nil {
		t.Fatalf("ParseListParams: %v", err)
	}
	result, err := svc.List(ctx, trainerID, params)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Pagination.Total < 1 {
		t.Fatalf("total = %d, want at least 1", result.Pagination.Total)
	}

	draftID := result.Items[0].ID
	name := "Archive Me"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	exStore := exercise.NewStore(pool)
	trainerProfileID, err := exStore.TrainerIDForUser(ctx, trainerID)
	if err != nil {
		t.Fatalf("TrainerIDForUser: %v", err)
	}
	catalogExercise, err := exStore.Create(ctx, trainerID, &trainerProfileID, exercise.UpsertInput{
		Name:        "Press",
		NameRu:      "Жим",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	detail, err := svc.Get(ctx, trainerID, draftID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	weekID := detail.Weeks[0].ID
	dayID := detail.Weeks[0].Days[0].ID
	sets, reps := "3", "10"
	if _, err := svc.Update(ctx, trainerID, draftID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, err := createSingleBlock(ctx, svc, trainerID, draftID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	if _, err := svc.Publish(ctx, trainerID, draftID); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	archived, err := svc.Archive(ctx, trainerID, draftID)
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if archived.Status != program.StatusArchived {
		t.Fatalf("status = %q", archived.Status)
	}

	republished, err := svc.Publish(ctx, trainerID, draftID)
	if err != nil {
		t.Fatalf("Publish() from archived: %v", err)
	}
	if republished.Status != program.StatusPublished {
		t.Fatalf("republished status = %q", republished.Status)
	}
}

func TestProgramService_weeksViaService(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-svc-weeks@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	week1ID := draft.Weeks[0].ID

	withWeek2, err := svc.AddWeek(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("AddWeek() error = %v", err)
	}
	week2ID := withWeek2.Weeks[1].ID

	reordered, err := svc.ReorderWeeks(ctx, trainerID, draft.ID, []uuid.UUID{week2ID, week1ID})
	if err != nil {
		t.Fatalf("ReorderWeeks() error = %v", err)
	}
	if reordered.Weeks[0].ID != week2ID {
		t.Fatalf("week order not updated")
	}

	week2 := reordered.Weeks[0]
	dayIDs := make([]uuid.UUID, len(week2.Days))
	for i, day := range week2.Days {
		dayIDs[i] = day.ID
	}
	if _, err := svc.ReorderDays(ctx, trainerID, draft.ID, week2ID, dayIDs); err != nil {
		t.Fatalf("ReorderDays() error = %v", err)
	}

	if _, err := svc.DeleteWeek(ctx, trainerID, draft.ID, week2ID); err != nil {
		t.Fatalf("DeleteWeek() error = %v", err)
	}
}

func TestProgramService_reorderAndDeleteGuards(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-svc-guards@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	weekID := draft.Weeks[0].ID

	withWeek2, err := svc.AddWeek(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("AddWeek() error = %v", err)
	}
	week2ID := withWeek2.Weeks[1].ID

	_, err = svc.ReorderDays(ctx, trainerID, draft.ID, uuid.New(), []uuid.UUID{uuid.New()})
	if !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("ReorderDays() error = %v, want ErrNotFound", err)
	}

	_, err = svc.ReorderBlockExercises(ctx, trainerID, draft.ID, uuid.New(), uuid.New(), nil)
	if !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("ReorderBlockExercises() error = %v, want ErrNotFound", err)
	}

	_, err = svc.DeleteWeek(ctx, trainerID, draft.ID, uuid.New())
	if !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("DeleteWeek() missing week error = %v, want ErrNotFound", err)
	}

	if _, err := svc.DeleteWeek(ctx, trainerID, draft.ID, week2ID); err != nil {
		t.Fatalf("DeleteWeek() week2 error = %v", err)
	}

	_, err = svc.DeleteWeek(ctx, trainerID, draft.ID, weekID)
	if !errors.Is(err, program.ErrLastWeek) {
		t.Fatalf("DeleteWeek() error = %v, want ErrLastWeek", err)
	}

	_, err = svc.AddDay(ctx, trainerID, draft.ID, uuid.New())
	if !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("AddDay() missing week error = %v, want ErrNotFound", err)
	}
}

func TestProgramService_publishWithTwoWeeks(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-svc-two-weeks@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerID, nil, exercise.UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	withWeek2, err := svc.AddWeek(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("AddWeek() error = %v", err)
	}

	name := "Two-week plan"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, trainerID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	sets, reps := "3", "10"
	week1ID := withWeek2.Weeks[0].ID
	week2ID := withWeek2.Weeks[1].ID
	day1ID := withWeek2.Weeks[0].Days[0].ID
	day2ID := withWeek2.Weeks[1].Days[0].ID

	if _, err := createSingleBlock(ctx, svc, trainerID, draft.ID, week1ID, day1ID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise week1 error = %v", err)
	}
	if _, err := createSingleBlock(ctx, svc, trainerID, draft.ID, week2ID, day2ID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise week2 error = %v", err)
	}

	published, err := svc.Publish(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if published.Status != program.StatusPublished {
		t.Fatalf("status = %q", published.Status)
	}
}

func TestProgramService_ClientProgramAssignment(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "assign-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "assign-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	q := sqlc.New(pool)
	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerUserID, nil, exercise.UpsertInput{
		Name:        "Row",
		NameRu:      "Тяга",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := "3", "10"
	if _, err := createSingleBlock(ctx, svc, trainerUserID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	name := "Client Program"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := svc.Publish(ctx, trainerUserID, draft.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	programID := draft.ID
	assignment, err := svc.SetClientProgramAssignment(ctx, trainerUserID, clientUserID, &programID)
	if err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}
	if assignment == nil {
		t.Fatal("expected assignment")
	}
	if assignment.ProgramID != programID {
		t.Fatalf("program_id = %v, want %v", assignment.ProgramID, programID)
	}
	if assignment.ClientPlanAt == nil {
		t.Fatal("expected client_plan_at")
	}

	got, err := svc.GetClientProgramAssignment(ctx, trainerUserID, clientUserID)
	if err != nil {
		t.Fatalf("GetClientProgramAssignment: %v", err)
	}
	if got == nil || got.ID != assignment.ID {
		t.Fatalf("GetClientProgramAssignment = %+v, want id %v", got, assignment.ID)
	}

	cleared, err := svc.SetClientProgramAssignment(ctx, trainerUserID, clientUserID, nil)
	if err != nil {
		t.Fatalf("clear assignment: %v", err)
	}
	if cleared != nil {
		t.Fatalf("clear assignment = %+v, want nil", cleared)
	}
	got, err = svc.GetClientProgramAssignment(ctx, trainerUserID, clientUserID)
	if err != nil {
		t.Fatalf("GetClientProgramAssignment after clear: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil assignment after clear, got %+v", got)
	}
}

func TestProgramService_BulkSetClientProgramAssignment(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "bulk-assign-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	linkedClientID, err := authStore.RegisterTrainerEmailPassword(ctx, "bulk-linked@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register linked client: %v", err)
	}
	unlinkedClientID, err := authStore.RegisterTrainerEmailPassword(ctx, "bulk-unlinked@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register unlinked client: %v", err)
	}

	q := sqlc.New(pool)
	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerID, linkedClientID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerUserID, nil, exercise.UpsertInput{
		Name:        "Press",
		NameRu:      "Жим",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := "3", "8"
	if _, err := createSingleBlock(ctx, svc, trainerUserID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	name := "Bulk Program"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := svc.Publish(ctx, trainerUserID, draft.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	programID := draft.ID
	result, err := svc.BulkSetClientProgramAssignment(ctx, trainerUserID, program.BulkSetClientProgramAssignmentRequest{
		ProgramID:     &programID,
		ClientUserIDs: []uuid.UUID{linkedClientID, unlinkedClientID, linkedClientID},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment: %v", err)
	}
	if len(result.Assigned) != 1 || result.Assigned[0].ClientUserID != linkedClientID {
		t.Fatalf("assigned = %+v", result.Assigned)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].ClientUserID != unlinkedClientID || result.Skipped[0].Reason != "not_linked" {
		t.Fatalf("skipped = %+v", result.Skipped)
	}

	clearResult, err := svc.BulkSetClientProgramAssignment(ctx, trainerUserID, program.BulkSetClientProgramAssignmentRequest{
		ClientUserIDs: []uuid.UUID{linkedClientID},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment clear: %v", err)
	}
	if len(clearResult.Cleared) != 1 || clearResult.Cleared[0] != linkedClientID {
		t.Fatalf("cleared = %+v", clearResult.Cleared)
	}
}

func TestProgramService_AssignmentSyncAndVersionCleanup(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "sync-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "sync-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	q := sqlc.New(pool)
	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerUserID, nil, exercise.UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := "3", "10"
	if _, err := createSingleBlock(ctx, svc, trainerUserID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	name := "Sync Program"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	published, err := svc.Publish(ctx, trainerUserID, draft.ID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	v1ID := *published.LatestProgramVersionID

	programID := draft.ID
	assignment, err := svc.SetClientProgramAssignment(ctx, trainerUserID, clientUserID, &programID)
	if err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	updatedName := "Sync Program v2"
	if _, err := svc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{Name: &updatedName}); err != nil {
		t.Fatalf("Update after publish: %v", err)
	}
	afterUpdate, err := svc.PublishUpdate(ctx, trainerUserID, draft.ID)
	if err != nil {
		t.Fatalf("PublishUpdate: %v", err)
	}
	v2ID := *afterUpdate.LatestProgramVersionID
	if v2ID == v1ID {
		t.Fatal("expected new version after publish-update")
	}

	list, err := svc.ListAssignments(ctx, trainerUserID, programID)
	if err != nil {
		t.Fatalf("ListAssignments: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("assignments = %d, want 1", len(list.Items))
	}
	if list.Items[0].ProgramVersionID != v1ID {
		t.Fatalf("assignment version = %v, want %v", list.Items[0].ProgramVersionID, v1ID)
	}
	if list.Items[0].IsBehindLatest == nil || !*list.Items[0].IsBehindLatest {
		t.Fatal("expected is_behind_latest true before sync")
	}

	if err := svc.DeleteVersion(ctx, trainerUserID, programID, v1ID); !errors.Is(err, program.ErrVersionHasAssignments) {
		t.Fatalf("DeleteVersion with assignments error = %v, want ErrVersionHasAssignments", err)
	}

	allActive := true
	synced, err := svc.SyncAssignments(ctx, trainerUserID, programID, program.AssignmentSyncRequest{AllActive: &allActive})
	if err != nil {
		t.Fatalf("SyncAssignments: %v", err)
	}
	if len(synced.Synced) != 1 {
		t.Fatalf("synced = %d, want 1", len(synced.Synced))
	}
	if synced.Synced[0].ProgramVersionID != v2ID {
		t.Fatalf("synced version = %v, want %v", synced.Synced[0].ProgramVersionID, v2ID)
	}

	versions, err := svc.ListVersions(ctx, trainerUserID, programID)
	if err != nil {
		t.Fatalf("ListVersions after auto cleanup: %v", err)
	}
	if len(versions.Items) != 1 {
		t.Fatalf("versions after sync auto cleanup = %d, want 1", len(versions.Items))
	}
	if versions.Items[0].ID != v2ID {
		t.Fatalf("remaining version = %v, want %v", versions.Items[0].ID, v2ID)
	}

	syncedAgain, err := svc.SyncAssignments(ctx, trainerUserID, programID, program.AssignmentSyncRequest{AllActive: &allActive})
	if err != nil {
		t.Fatalf("SyncAssignments again: %v", err)
	}
	if len(syncedAgain.Skipped) != 1 || syncedAgain.Skipped[0].Reason != "already_on_latest" {
		t.Fatalf("second sync skipped = %+v, want already_on_latest", syncedAgain.Skipped)
	}

	cleanup, err := svc.CleanupVersions(ctx, trainerUserID, programID)
	if err != nil {
		t.Fatalf("CleanupVersions: %v", err)
	}
	if len(cleanup.DeletedVersionIDs) != 0 {
		t.Fatalf("deleted versions = %v, want none", cleanup.DeletedVersionIDs)
	}
	if len(cleanup.Skipped) != 1 || cleanup.Skipped[0].Reason != "sole_version" {
		t.Fatalf("cleanup skipped = %+v, want sole_version", cleanup.Skipped)
	}

	versionsAfter, err := svc.ListVersions(ctx, trainerUserID, programID)
	if err != nil {
		t.Fatalf("ListVersions after cleanup: %v", err)
	}
	if len(versionsAfter.Items) != 1 {
		t.Fatalf("versions after cleanup = %d, want 1", len(versionsAfter.Items))
	}
	if versionsAfter.Items[0].ID != v2ID {
		t.Fatalf("remaining version = %v, want %v", versionsAfter.Items[0].ID, v2ID)
	}

	if err := svc.DeleteVersion(ctx, trainerUserID, programID, v2ID); !errors.Is(err, program.ErrSoleProgramVersion) {
		t.Fatalf("DeleteVersion sole error = %v, want ErrSoleProgramVersion", err)
	}

	assignmentID := assignment.ID
	syncByID, err := svc.SyncAssignments(ctx, trainerUserID, programID, program.AssignmentSyncRequest{
		AssignmentIDs: []uuid.UUID{assignmentID},
	})
	if err != nil {
		t.Fatalf("SyncAssignments by id: %v", err)
	}
	if len(syncByID.Skipped) != 1 || syncByID.Skipped[0].Reason != "already_on_latest" {
		t.Fatalf("sync by id skipped = %+v", syncByID.Skipped)
	}

	missingID := uuid.New()
	syncMissing, err := svc.SyncAssignments(ctx, trainerUserID, programID, program.AssignmentSyncRequest{
		AssignmentIDs: []uuid.UUID{missingID},
	})
	if err != nil {
		t.Fatalf("SyncAssignments missing id: %v", err)
	}
	if len(syncMissing.Skipped) != 1 || syncMissing.Skipped[0].Reason != "not_found" {
		t.Fatalf("sync missing skipped = %+v, want not_found", syncMissing.Skipped)
	}
}

func TestProgramService_CleanupVersions_skipsAssignedVersion(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "cleanup-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "cleanup-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	q := sqlc.New(pool)
	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerUserID, nil, exercise.UpsertInput{
		Name:        "Bench",
		NameRu:      "Жим",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupChest,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := "3", "10"
	if _, err := createSingleBlock(ctx, svc, trainerUserID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	name := "Cleanup Program"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	published, err := svc.Publish(ctx, trainerUserID, draft.ID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	v1ID := *published.LatestProgramVersionID
	programID := draft.ID

	if _, err := svc.SetClientProgramAssignment(ctx, trainerUserID, clientUserID, &programID); err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}

	updatedName := "Cleanup Program v2"
	if _, err := svc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{Name: &updatedName}); err != nil {
		t.Fatalf("Update after publish: %v", err)
	}
	afterUpdate, err := svc.PublishUpdate(ctx, trainerUserID, draft.ID)
	if err != nil {
		t.Fatalf("PublishUpdate: %v", err)
	}
	v2ID := *afterUpdate.LatestProgramVersionID

	cleanup, err := svc.CleanupVersions(ctx, trainerUserID, programID)
	if err != nil {
		t.Fatalf("CleanupVersions: %v", err)
	}
	if len(cleanup.DeletedVersionIDs) != 1 || cleanup.DeletedVersionIDs[0] != v2ID {
		t.Fatalf("deleted versions = %v, want [%v]", cleanup.DeletedVersionIDs, v2ID)
	}
	if len(cleanup.Skipped) != 1 || cleanup.Skipped[0].VersionID != v1ID || cleanup.Skipped[0].Reason != "has_assignments" {
		t.Fatalf("skipped = %+v, want v1 has_assignments", cleanup.Skipped)
	}
}

func TestProgramService_SetClientAssignment_notLinked(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "notlinked-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "notlinked-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	svc := program.NewService(pool)
	programID := uuid.New()
	_, err = svc.SetClientProgramAssignment(ctx, trainerUserID, clientUserID, &programID)
	if !errors.Is(err, program.ErrClientNotLinked) {
		t.Fatalf("SetClientProgramAssignment() error = %v, want ErrClientNotLinked", err)
	}
}

func TestProgramService_GetClientAssignment_notLinked(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "getlink-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "getlink-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	svc := program.NewService(pool)
	_, err = svc.GetClientProgramAssignment(ctx, trainerUserID, clientUserID)
	if !errors.Is(err, program.ErrClientNotLinked) {
		t.Fatalf("GetClientProgramAssignment() error = %v, want ErrClientNotLinked", err)
	}
}

func TestProgramStore_assignmentQueriesWithoutAssignment(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "store-assign-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "store-assign-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	q := sqlc.New(pool)
	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	store := program.NewStore(pool)
	list, err := store.ListProgramAssignments(ctx, draft.ID)
	if err != nil {
		t.Fatalf("ListProgramAssignments: %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("assignments = %d, want 0", len(list.Items))
	}

	got, err := store.GetClientProgramAssignment(ctx, pgconv.FromPGUUID(trainerID), clientUserID)
	if err != nil {
		t.Fatalf("GetClientProgramAssignment: %v", err)
	}
	if got != nil {
		t.Fatalf("assignment = %+v, want nil", got)
	}
}

func TestProgramService_SetClientAssignment_draftProgram(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "draft-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "draft-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	q := sqlc.New(pool)
	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.SetClientProgramAssignment(ctx, trainerUserID, clientUserID, &draft.ID)
	if !errors.Is(err, program.ErrProgramNotPublished) {
		t.Fatalf("SetClientProgramAssignment() error = %v, want ErrProgramNotPublished", err)
	}
}

func TestProgramService_SetClientAssignment_wrongOwner(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	ownerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "owner-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	otherUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "other-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register other: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "wrongowner-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	q := sqlc.New(pool)
	otherTrainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(otherUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, otherTrainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	ownerSvc := program.NewService(pool)
	draft, err := ownerSvc.Create(ctx, ownerUserID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, ownerUserID, nil, exercise.UpsertInput{
		Name:        "Row",
		NameRu:      "Тяга",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}
	sets, reps := "3", "10"
	if _, err := ownerSvc.CreateDayBlock(ctx, ownerUserID, draft.ID, weekID, dayID, program.CreateDayBlockInput{
		BlockType: program.BlockTypeSingle,
		Exercise: &program.DayExerciseInput{
			ExerciseID: catalogExercise.ID,
			Sets:       &sets,
			Reps:       &reps,
		},
	}); err != nil {
		t.Fatalf("CreateDayBlock: %v", err)
	}
	name := "Owner Program"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := ownerSvc.Update(ctx, ownerUserID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	published, err := ownerSvc.Publish(ctx, ownerUserID, draft.ID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published.LatestProgramVersionID == nil {
		t.Fatal("expected version after publish")
	}

	otherSvc := program.NewService(pool)
	_, err = otherSvc.SetClientProgramAssignment(ctx, otherUserID, clientUserID, &draft.ID)
	if !errors.Is(err, program.ErrForbidden) {
		t.Fatalf("SetClientProgramAssignment() error = %v, want ErrForbidden", err)
	}
}

func TestProgramService_SetClientAssignment_blockedClient(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "blocked-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "blocked-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}

	q := sqlc.New(pool)
	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'blocked')
	`, trainerID, clientUserID); err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	svc := program.NewService(pool)
	programID := uuid.New()
	_, err = svc.SetClientProgramAssignment(ctx, trainerUserID, clientUserID, &programID)
	if !errors.Is(err, program.ErrClientBlocked) {
		t.Fatalf("SetClientProgramAssignment() error = %v, want ErrClientBlocked", err)
	}
}

func TestProgramService_adminCannotSyncOtherUsersProgram(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	ownerID, err := authStore.RegisterTrainerEmailPassword(ctx, "sync-owner@test.com", pwHash, "Owner")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	adminID, err := authStore.RegisterTrainerEmailPassword(ctx, "sync-admin@test.com", pwHash, "Admin")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	promoteToAdminOnly(t, pool, adminID)

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, ownerID, nil, exercise.UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, ownerID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := "3", "10"
	if _, err := createSingleBlock(ctx, svc, ownerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	name := "Owner Program"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, ownerID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := svc.Publish(ctx, ownerID, draft.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	allActive := true
	_, err = svc.SyncAssignments(ctx, adminID, draft.ID, program.AssignmentSyncRequest{AllActive: &allActive})
	if !errors.Is(err, program.ErrForbidden) {
		t.Fatalf("SyncAssignments() by admin error = %v, want ErrForbidden", err)
	}
}
