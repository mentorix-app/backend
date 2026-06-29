//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

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
	withExercise, err := progStore.AddDayExercise(ctx, trainerID, draft.ID, dayID, program.DayExerciseInput{
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

	withDay, err := progStore.AddDay(ctx, draft.ID)
	if err != nil {
		t.Fatalf("AddDay: %v", err)
	}
	if len(withDay.Days) != 2 {
		t.Fatalf("days = %d, want 2", len(withDay.Days))
	}
	dayID := withDay.Days[1].ID

	sets, reps := 4, 8
	withExercise, err := progStore.AddDayExercise(ctx, trainerID, draft.ID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	itemID := withExercise.Days[1].Exercises[0].ID

	newSets := 5
	updatedExercise, err := progStore.UpdateDayExercise(ctx, trainerID, draft.ID, dayID, itemID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &newSets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("UpdateDayExercise: %v", err)
	}
	if *updatedExercise.Days[1].Exercises[0].Sets != newSets {
		t.Fatalf("sets = %d, want %d", *updatedExercise.Days[1].Exercises[0].Sets, newSets)
	}

	withoutExercise, err := progStore.DeleteDayExercise(ctx, draft.ID, dayID, itemID)
	if err != nil {
		t.Fatalf("DeleteDayExercise: %v", err)
	}
	if len(withoutExercise.Days[1].Exercises) != 0 {
		t.Fatalf("expected day exercise removed")
	}

	oneDay, err := progStore.DeleteDay(ctx, draft.ID, dayID)
	if err != nil {
		t.Fatalf("DeleteDay: %v", err)
	}
	if len(oneDay.Days) != 1 {
		t.Fatalf("days = %d, want 1", len(oneDay.Days))
	}

	if err := progStore.SoftDelete(ctx, draft.ID, trainerID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
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
