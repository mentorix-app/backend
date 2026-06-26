//go:build integration

package storetest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/health"
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

func TestExerciseStore_GetUpdateDelete(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := authStore.RegisterTrainerEmailPassword(ctx, "exercise-crud@test.com", pwHash)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	store := exercise.NewStore(pool)
	created, err := store.Create(ctx, userID, exercise.UpsertInput{
		Name:        "Deadlift",
		NameRu:      "Становая",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack,
		Difficulty:  exercise.DifficultyIntermediate,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != "Deadlift" {
		t.Errorf("name = %q", got.Name)
	}

	newName := "Romanian Deadlift"
	updated, err := store.Update(ctx, created.ID, userID, exercise.UpsertInput{
		Name:        newName,
		NameRu:      got.NameRu,
		Type:        got.Type,
		MuscleGroup: got.MuscleGroup,
		Difficulty:  got.Difficulty,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != newName {
		t.Errorf("updated name = %q", updated.Name)
	}

	n, err := store.DeleteMany(ctx, []uuid.UUID{created.ID})
	if err != nil {
		t.Fatalf("DeleteMany() error = %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
}

func TestExerciseStore_GetByIDNotFound(t *testing.T) {
	pool := NewPool(t)
	store := exercise.NewStore(pool)
	_, err := store.GetByID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestExerciseStore_ListWithFilters(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := authStore.RegisterTrainerEmailPassword(ctx, "exercise-filter@test.com", pwHash)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	store := exercise.NewStore(pool)
	equipment := exercise.EquipmentBarbell
	_, err = store.Create(ctx, userID, exercise.UpsertInput{
		Name:        "Barbell Row",
		NameRu:      "Тяга",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupBack,
		Difficulty:  exercise.DifficultyIntermediate,
		Equipment:   &equipment,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	params, err := exercise.ParseListParams("1", "20", "name", "asc", "row", "strength", "back", "intermediate", "barbell")
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
}

func TestHealthRegisterReady_allDependenciesOK(t *testing.T) {
	pool := NewPool(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	e := echo.New()
	health.RegisterReady(e, pool, rdb)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp health.ReadyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != health.ReadyStatusReady {
		t.Fatalf("status = %q, want %q", resp.Status, health.ReadyStatusReady)
	}
	if resp.Checks["database"] != health.StatusOK || resp.Checks["redis"] != health.StatusOK {
		t.Fatalf("checks = %+v", resp.Checks)
	}
}
