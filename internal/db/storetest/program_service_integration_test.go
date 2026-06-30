//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/auth"
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

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := 3, 12
	weight := 60.0
	instruction := "controlled"
	withExercise, err := svc.AddDayExercise(ctx, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID:  catalogExercise.ID,
		Sets:        &sets,
		Reps:        &reps,
		WeightKg:    &weight,
		Instruction: &instruction,
	})
	if err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	itemID := withExercise.Weeks[0].Days[0].Exercises[0].ID

	newSets := 4
	updated, err := svc.UpdateDayExercise(ctx, trainerID, draft.ID, weekID, dayID, itemID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &newSets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("UpdateDayExercise() error = %v", err)
	}
	if *updated.Weeks[0].Days[0].Exercises[0].Sets != newSets {
		t.Fatalf("sets = %d, want %d", *updated.Weeks[0].Days[0].Exercises[0].Sets, newSets)
	}

	without, err := svc.DeleteDayExercise(ctx, trainerID, draft.ID, weekID, dayID, itemID)
	if err != nil {
		t.Fatalf("DeleteDayExercise() error = %v", err)
	}
	if len(without.Weeks[0].Days[0].Exercises) != 0 {
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

	withExercise, err = svc.AddDayExercise(ctx, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID:  catalogExercise.ID,
		Sets:        &sets,
		Reps:        &reps,
		WeightKg:    &weight,
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

	detail, err := svc.Get(ctx, trainerID, draftID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	weekID := detail.Weeks[0].ID
	dayID := detail.Weeks[0].Days[0].ID
	sets, reps := 3, 10
	if _, err := svc.Update(ctx, trainerID, draftID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, err := svc.AddDayExercise(ctx, trainerID, draftID, weekID, dayID, program.DayExerciseInput{
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

	_, err = svc.ReorderWeekExercises(ctx, trainerID, draft.ID, uuid.New(), nil)
	if !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("ReorderWeekExercises() error = %v, want ErrNotFound", err)
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
	catalogExercise, err := exStore.Create(ctx, trainerID, exercise.UpsertInput{
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

	sets, reps := 3, 10
	week1ID := withWeek2.Weeks[0].ID
	week2ID := withWeek2.Weeks[1].ID
	day1ID := withWeek2.Weeks[0].Days[0].ID
	day2ID := withWeek2.Weeks[1].Days[0].ID

	if _, err := svc.AddDayExercise(ctx, trainerID, draft.ID, week1ID, day1ID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("AddDayExercise week1 error = %v", err)
	}
	if _, err := svc.AddDayExercise(ctx, trainerID, draft.ID, week2ID, day2ID, program.DayExerciseInput{
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
