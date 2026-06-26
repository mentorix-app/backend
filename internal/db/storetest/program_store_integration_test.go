//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"

	"mentorix-backend/internal/auth"
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
	trainerID, err := authStore.RegisterTrainerEmailPassword(ctx, "program-store@test.com", pwHash)
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
	if len(draft.Days) != 1 {
		t.Fatalf("days len = %d, want 1", len(draft.Days))
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

	dayID := draft.Days[0].ID
	sets, reps := 3, 10
	withExercise, err := progStore.AddDayExercise(ctx, draft.ID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	if len(withExercise.Days[0].Exercises) != 1 {
		t.Fatalf("day exercises len = %d, want 1", len(withExercise.Days[0].Exercises))
	}

	published, err := svc.Publish(ctx, trainerID, draft.ID)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if published.Status != program.StatusPublished {
		t.Errorf("status = %q, want published", published.Status)
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
	trainerA, err := authStore.RegisterTrainerEmailPassword(ctx, "trainer-a@test.com", pwHash)
	if err != nil {
		t.Fatalf("register trainer A: %v", err)
	}
	trainerB, err := authStore.RegisterTrainerEmailPassword(ctx, "trainer-b@test.com", pwHash)
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
