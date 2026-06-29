//go:build integration

package storetest

import (
	"context"
	"testing"

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
	dayID := draft.Days[0].ID
	sets, reps := 3, 12
	weight := 60.0
	instruction := "controlled"
	withExercise, err := svc.AddDayExercise(ctx, trainerID, draft.ID, dayID, program.DayExerciseInput{
		ExerciseID:  catalogExercise.ID,
		Sets:        &sets,
		Reps:        &reps,
		WeightKg:    &weight,
		Instruction: &instruction,
	})
	if err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	itemID := withExercise.Days[0].Exercises[0].ID

	newSets := 4
	updated, err := svc.UpdateDayExercise(ctx, trainerID, draft.ID, dayID, itemID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &newSets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("UpdateDayExercise() error = %v", err)
	}
	if *updated.Days[0].Exercises[0].Sets != newSets {
		t.Fatalf("sets = %d, want %d", *updated.Days[0].Exercises[0].Sets, newSets)
	}

	without, err := svc.DeleteDayExercise(ctx, trainerID, draft.ID, dayID, itemID)
	if err != nil {
		t.Fatalf("DeleteDayExercise() error = %v", err)
	}
	if len(without.Days[0].Exercises) != 0 {
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

	withExercise, err = svc.AddDayExercise(ctx, trainerID, draft.ID, dayID, program.DayExerciseInput{
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
	dayID := detail.Days[0].ID
	sets, reps := 3, 10
	if _, err := svc.Update(ctx, trainerID, draftID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, err := svc.AddDayExercise(ctx, trainerID, draftID, dayID, program.DayExerciseInput{
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
