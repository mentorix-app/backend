//go:build integration

package storetest

import (
	"context"
	"testing"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
)

func TestExerciseStore_CreateAndList(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := authStore.RegisterTrainerEmailPassword(ctx, "exercise-store@test.com", pwHash)
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	store := exercise.NewStore(pool)
	created, err := store.Create(ctx, userID, exercise.UpsertInput{
		Name:        "Back Squat",
		NameRu:      "Присед",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Name != "Back Squat" {
		t.Errorf("name = %q, want Back Squat", created.Name)
	}

	params, err := exercise.ParseListParams("1", "20", "name", "asc", "squat", "", "", "", "")
	if err != nil {
		t.Fatalf("ParseListParams() error = %v", err)
	}
	result, err := store.List(ctx, params)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Pagination.Total < 1 {
		t.Fatalf("total = %d, want at least 1", result.Pagination.Total)
	}
	found := false
	for _, item := range result.Items {
		if item.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created exercise not found in list with q=squat")
	}
}
