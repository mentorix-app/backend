//go:build integration

package storetest

import (
	"context"
	"errors"
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
	withExercise, err := progStore.AddDayExercise(ctx, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	if len(withExercise.Weeks[0].Days[0].Exercises) != 1 {
		t.Fatalf("day exercises len = %d, want 1", len(withExercise.Weeks[0].Days[0].Exercises))
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
	withExercise, err := progStore.AddDayExercise(ctx, trainerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise: %v", err)
	}
	var itemID uuid.UUID
	for _, day := range withExercise.Weeks[0].Days {
		if day.ID == dayID && len(day.Exercises) > 0 {
			itemID = day.Exercises[0].ID
			break
		}
	}
	if itemID == uuid.Nil {
		t.Fatal("expected exercise on day")
	}

	newSets := 5
	updatedExercise, err := progStore.UpdateDayExercise(ctx, trainerID, draft.ID, weekID, dayID, itemID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &newSets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("UpdateDayExercise: %v", err)
	}
	var updatedSets int
	for _, day := range updatedExercise.Weeks[0].Days {
		if day.ID == dayID && len(day.Exercises) > 0 {
			updatedSets = *day.Exercises[0].Sets
			break
		}
	}
	if updatedSets != newSets {
		t.Fatalf("sets = %d, want %d", updatedSets, newSets)
	}

	withoutExercise, err := progStore.DeleteDayExercise(ctx, draft.ID, weekID, dayID, itemID)
	if err != nil {
		t.Fatalf("DeleteDayExercise: %v", err)
	}
	var exercisesLeft int
	for _, day := range withoutExercise.Weeks[0].Days {
		if day.ID == dayID {
			exercisesLeft = len(day.Exercises)
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
	withExA, err := progStore.AddDayExercise(ctx, trainerID, draft.ID, week2ID, dayA, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise dayA: %v", err)
	}
	var itemA uuid.UUID
	for _, day := range withExA.Weeks[0].Days {
		if day.ID == dayA && len(day.Exercises) > 0 {
			itemA = day.Exercises[0].ID
			break
		}
	}
	if itemA == uuid.Nil {
		t.Fatal("expected exercise on dayA")
	}

	withExB, err := progStore.AddDayExercise(ctx, trainerID, draft.ID, week2ID, dayB, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err != nil {
		t.Fatalf("AddDayExercise dayB: %v", err)
	}
	var itemB uuid.UUID
	for _, day := range withExB.Weeks[0].Days {
		if day.ID == dayB && len(day.Exercises) > 0 {
			itemB = day.Exercises[0].ID
			break
		}
	}
	if itemB == uuid.Nil {
		t.Fatal("expected exercise on dayB")
	}

	_, err = progStore.ReorderWeekExercises(ctx, trainerID, draft.ID, week2ID, []program.WeekExerciseReorderDay{
		{DayID: uuid.New(), ExerciseItemIDs: []uuid.UUID{itemA}},
	})
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderWeekExercises foreign day error = %v, want ErrInvalidReorder", err)
	}

	_, err = progStore.ReorderWeekExercises(ctx, trainerID, draft.ID, week2ID, []program.WeekExerciseReorderDay{
		{DayID: dayB, ExerciseItemIDs: []uuid.UUID{uuid.New()}},
	})
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderWeekExercises unknown item error = %v, want ErrInvalidReorder", err)
	}

	moved, err := progStore.ReorderWeekExercises(ctx, trainerID, draft.ID, week2ID, []program.WeekExerciseReorderDay{
		{DayID: dayB, ExerciseItemIDs: []uuid.UUID{itemA, itemB}},
	})
	if err != nil {
		t.Fatalf("ReorderWeekExercises: %v", err)
	}
	var dayBExercises int
	for _, day := range moved.Weeks[0].Days {
		if day.ID == dayB {
			dayBExercises = len(day.Exercises)
			break
		}
	}
	if dayBExercises != 2 {
		t.Fatalf("dayB exercises = %d, want 2", dayBExercises)
	}

	_, err = progStore.ReorderWeekExercises(ctx, trainerID, draft.ID, week2ID, []program.WeekExerciseReorderDay{
		{DayID: dayB, ExerciseItemIDs: []uuid.UUID{itemA}},
	})
	if !errors.Is(err, program.ErrInvalidReorder) {
		t.Fatalf("ReorderWeekExercises partial list error = %v, want ErrInvalidReorder", err)
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
