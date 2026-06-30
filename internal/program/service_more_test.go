package program

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/exercise"
)

func TestService_Create(t *testing.T) {
	userID := uuid.New()
	want := Detail{Program: Program{ID: uuid.New(), Status: StatusDraft}}
	svc := testService(&fakeProgramStore{createDetail: want}, nil)
	got, err := svc.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("id = %v, want %v", got.ID, want.ID)
	}
}

func TestService_Update_forbiddenForOtherTrainer(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: ownerID, Status: StatusDraft},
	}, &fakeRoleQuerier{isAdmin: false})
	_, err := svc.Update(context.Background(), otherID, programID, UpdateInput{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Update() error = %v, want ErrForbidden", err)
	}
}

func TestService_Archive_fromPublished(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		detail:  Detail{Program: Program{ID: programID, Status: StatusArchived}},
	}, nil)
	got, err := svc.Archive(context.Background(), userID, programID)
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if got.Status != StatusArchived {
		t.Errorf("status = %q", got.Status)
	}
}

func TestService_AddDay_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	want := Detail{Program: Program{ID: programID, Status: StatusDraft}}
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  want,
	}, nil)
	got, err := svc.AddDay(context.Background(), userID, programID, uuid.New())
	if err != nil {
		t.Fatalf("AddDay() error = %v", err)
	}
	if got.ID != programID {
		t.Errorf("id = %v", got.ID)
	}
}

func TestService_Get_adminAccess(t *testing.T) {
	ownerID := uuid.New()
	adminID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: ownerID},
		detail:  Detail{Program: Program{ID: programID, CreatedBy: ownerID}},
	}, &fakeRoleQuerier{isAdmin: true})
	got, err := svc.Get(context.Background(), adminID, programID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != programID {
		t.Errorf("id = %v", got.ID)
	}
}

func TestService_Update_adminCanUpdateOtherUsersProgram(t *testing.T) {
	ownerID := uuid.New()
	adminID := uuid.New()
	programID := uuid.New()
	name := "Admin edit"
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: ownerID, Status: StatusDraft},
		detail:  Detail{Program: Program{ID: programID, CreatedBy: ownerID, Name: name}},
	}, &fakeRoleQuerier{isAdmin: true})
	got, err := svc.Update(context.Background(), adminID, programID, UpdateInput{Name: &name})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.Name != name {
		t.Errorf("name = %q", got.Name)
	}
}

func TestService_Delete_adminCanDeleteOtherUsersProgram(t *testing.T) {
	ownerID := uuid.New()
	adminID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: ownerID, Status: StatusDraft},
	}, &fakeRoleQuerier{isAdmin: true})
	if err := svc.Delete(context.Background(), adminID, programID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestService_Delete_softDeletesDraft(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}, nil)
	if err := svc.Delete(context.Background(), userID, programID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

type setStatusErrStore struct {
	fakeProgramStore
	setStatusErr error
}

func (s *setStatusErrStore) SetStatus(context.Context, uuid.UUID, uuid.UUID, Status) (Detail, error) {
	return Detail{}, s.setStatusErr
}

func TestService_Publish_setStatusNotFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10
	svc := testService(&setStatusErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
			detail: Detail{
				Program: Program{
					ID: programID, CreatedBy: userID, Status: StatusDraft,
					Name: "Program", Category: &category, Difficulty: &difficulty,
				},
				Weeks: []Week{publishableWeek([]DayExercise{{
					ExerciseID: uuid.New(),
					Sets:       &sets,
					Reps:       &reps,
				}})},
			},
		},
		setStatusErr: pgx.ErrNoRows,
	}, nil)
	_, err := svc.Publish(context.Background(), userID, programID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Publish() error = %v, want ErrNotFound", err)
	}
}

func TestService_Archive_deletedProgram(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	deleted := time.Now().UTC()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished, DeletedAt: &deleted},
	}, nil)
	_, err := svc.Archive(context.Background(), userID, programID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Archive() error = %v, want ErrNotFound", err)
	}
}

func TestService_Publish_fromArchived(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusArchived},
		detail: Detail{
			Program: Program{
				ID: programID, CreatedBy: userID, Status: StatusArchived,
				Name: "Program", Category: &category, Difficulty: &difficulty,
			},
			Weeks: []Week{publishableWeek([]DayExercise{{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			}})},
		},
	}, nil)
	got, err := svc.Publish(context.Background(), userID, programID)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got.Status != StatusPublished {
		t.Errorf("status = %q, want %q", got.Status, StatusPublished)
	}
}

func TestService_Publish_deletedDetail(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	deleted := time.Now().UTC()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail: Detail{
			Program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft, DeletedAt: &deleted},
		},
	}, nil)
	_, err := svc.Publish(context.Background(), userID, programID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Publish() error = %v, want ErrNotFound", err)
	}
}

func TestService_Publish_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail: Detail{
			Program: Program{
				ID: programID, CreatedBy: userID, Status: StatusDraft,
				Name: "Program", Category: &category, Difficulty: &difficulty,
			},
			Weeks: []Week{publishableWeek([]DayExercise{{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			}})},
		},
	}, nil)
	got, err := svc.Publish(context.Background(), userID, programID)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got.Status != StatusPublished {
		t.Errorf("status = %q", got.Status)
	}
}

func TestService_Archive_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		detail:  Detail{Program: Program{ID: programID, Status: StatusArchived}},
	}, nil)
	got, err := svc.Archive(context.Background(), userID, programID)
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if got.Status != StatusArchived {
		t.Errorf("status = %q", got.Status)
	}
}

func TestService_Update_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	name := "Updated"
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  Detail{Program: Program{ID: programID, Name: name}},
	}, nil)
	got, err := svc.Update(context.Background(), userID, programID, UpdateInput{Name: &name})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.Name != name {
		t.Errorf("name = %q", got.Name)
	}
}

func TestService_Get_notFound(t *testing.T) {
	svc := testService(&fakeProgramStore{err: pgx.ErrNoRows}, nil)
	_, err := svc.Get(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestService_Delete_notFound(t *testing.T) {
	svc := testService(&fakeProgramStore{err: pgx.ErrNoRows}, nil)
	err := svc.Delete(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestService_Publish_notFound(t *testing.T) {
	svc := testService(&fakeProgramStore{err: pgx.ErrNoRows}, nil)
	_, err := svc.Publish(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Publish() error = %v, want ErrNotFound", err)
	}
}

func TestService_Archive_invalidTransition(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}, nil)
	_, err := svc.Archive(context.Background(), userID, programID)
	if !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("Archive() error = %v, want ErrInvalidStatusTransition", err)
	}
}

func TestService_dayExerciseOperations(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	dayID := uuid.New()
	itemID := uuid.New()
	sets, reps := 3, 10
	detail := Detail{Program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft}}
	store := &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  detail,
	}
	svc := testService(store, nil)
	in := DayExerciseInput{ExerciseID: uuid.New(), Sets: &sets, Reps: &reps}

	if _, err := svc.AddDayExercise(context.Background(), userID, programID, weekID, dayID, in); err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	if _, err := svc.UpdateDayExercise(context.Background(), userID, programID, weekID, dayID, itemID, in); err != nil {
		t.Fatalf("UpdateDayExercise() error = %v", err)
	}
	if _, err := svc.DeleteDayExercise(context.Background(), userID, programID, weekID, dayID, itemID); err != nil {
		t.Fatalf("DeleteDayExercise() error = %v", err)
	}
	if _, err := svc.DeleteDay(context.Background(), userID, programID, weekID, dayID); err != nil {
		t.Fatalf("DeleteDay() error = %v", err)
	}
}

func TestService_AddDayExercise_validationError(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}, nil)
	_, err := svc.AddDayExercise(context.Background(), userID, programID, uuid.New(), uuid.New(), DayExerciseInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

type deleteDayErrStore struct {
	fakeProgramStore
	deleteDayErr error
}

func (d *deleteDayErrStore) DeleteDay(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (Detail, error) {
	return Detail{}, d.deleteDayErr
}

func TestService_DeleteDay_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&deleteDayErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		deleteDayErr: pgx.ErrNoRows,
	}, nil)
	_, err := svc.DeleteDay(context.Background(), userID, programID, uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteDay() error = %v, want ErrNotFound", err)
	}
}

func TestService_AddWeek_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	want := Detail{Program: Program{ID: programID, Status: StatusDraft}}
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  want,
	}, nil)
	got, err := svc.AddWeek(context.Background(), userID, programID)
	if err != nil {
		t.Fatalf("AddWeek() error = %v", err)
	}
	if got.ID != programID {
		t.Errorf("id = %v", got.ID)
	}
}

func TestService_DeleteWeek_lastWeek(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	svc := testService(&deleteWeekErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		deleteWeekErr: ErrLastWeek,
	}, nil)
	_, err := svc.DeleteWeek(context.Background(), userID, programID, weekID)
	if !errors.Is(err, ErrLastWeek) {
		t.Fatalf("DeleteWeek() error = %v, want ErrLastWeek", err)
	}
}

func TestService_ReorderWeeks_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	want := Detail{Program: Program{ID: programID, Status: StatusDraft}}
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  want,
	}, nil)
	got, err := svc.ReorderWeeks(context.Background(), userID, programID, []uuid.UUID{weekID})
	if err != nil {
		t.Fatalf("ReorderWeeks() error = %v", err)
	}
	if got.ID != programID {
		t.Errorf("id = %v", got.ID)
	}
}

func TestService_ReorderDays_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	dayID := uuid.New()
	want := Detail{Program: Program{ID: programID, Status: StatusDraft}}
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  want,
	}, nil)
	got, err := svc.ReorderDays(context.Background(), userID, programID, weekID, []uuid.UUID{dayID})
	if err != nil {
		t.Fatalf("ReorderDays() error = %v", err)
	}
	if got.ID != programID {
		t.Errorf("id = %v", got.ID)
	}
}

func TestService_ReorderWeekExercises_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	dayID := uuid.New()
	itemID := uuid.New()
	want := Detail{Program: Program{ID: programID, Status: StatusDraft}}
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  want,
	}, nil)
	got, err := svc.ReorderWeekExercises(context.Background(), userID, programID, weekID, []WeekExerciseReorderDay{{
		DayID:           dayID,
		ExerciseItemIDs: []uuid.UUID{itemID},
	}})
	if err != nil {
		t.Fatalf("ReorderWeekExercises() error = %v", err)
	}
	if got.ID != programID {
		t.Errorf("id = %v", got.ID)
	}
}

type deleteWeekErrStore struct {
	fakeProgramStore
	deleteWeekErr error
}

func (d *deleteWeekErrStore) DeleteWeek(context.Context, uuid.UUID, uuid.UUID) (Detail, error) {
	return Detail{}, d.deleteWeekErr
}

func TestService_DeleteWeek_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&deleteWeekErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		deleteWeekErr: pgx.ErrNoRows,
	}, nil)
	_, err := svc.DeleteWeek(context.Background(), userID, programID, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteWeek() error = %v, want ErrNotFound", err)
	}
}

func TestService_ReorderWeeks_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&reorderWeeksErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: ErrInvalidReorder,
	}, nil)
	_, err := svc.ReorderWeeks(context.Background(), userID, programID, []uuid.UUID{uuid.New()})
	if !errors.Is(err, ErrInvalidReorder) {
		t.Fatalf("ReorderWeeks() error = %v, want ErrInvalidReorder", err)
	}
}

type reorderWeeksErrStore struct {
	fakeProgramStore
	reorderErr error
}

func (r *reorderWeeksErrStore) ReorderWeeks(context.Context, uuid.UUID, []uuid.UUID) (Detail, error) {
	return Detail{}, r.reorderErr
}

type addDayErrStore struct {
	fakeProgramStore
	addDayErr error
}

func (a *addDayErrStore) AddDay(context.Context, uuid.UUID, uuid.UUID) (Detail, error) {
	return Detail{}, a.addDayErr
}

type reorderDaysErrStore struct {
	fakeProgramStore
	reorderErr error
}

func (r *reorderDaysErrStore) ReorderDays(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) (Detail, error) {
	return Detail{}, r.reorderErr
}

type reorderExercisesErrStore struct {
	fakeProgramStore
	reorderErr error
}

func (r *reorderExercisesErrStore) ReorderWeekExercises(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, []WeekExerciseReorderDay) (Detail, error) {
	return Detail{}, r.reorderErr
}

func TestService_AddDay_maxDays(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&addDayErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		addDayErr: ErrMaxDaysPerWeek,
	}, nil)
	_, err := svc.AddDay(context.Background(), userID, programID, uuid.New())
	if !errors.Is(err, ErrMaxDaysPerWeek) {
		t.Fatalf("AddDay() error = %v, want ErrMaxDaysPerWeek", err)
	}
}

func TestService_AddDay_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&addDayErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		addDayErr: pgx.ErrNoRows,
	}, nil)
	_, err := svc.AddDay(context.Background(), userID, programID, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("AddDay() error = %v, want ErrNotFound", err)
	}
}

func TestService_ReorderDays_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&reorderDaysErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: pgx.ErrNoRows,
	}, nil)
	_, err := svc.ReorderDays(context.Background(), userID, programID, uuid.New(), []uuid.UUID{uuid.New()})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ReorderDays() error = %v, want ErrNotFound", err)
	}
}

func TestService_ReorderWeekExercises_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&reorderExercisesErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: pgx.ErrNoRows,
	}, nil)
	_, err := svc.ReorderWeekExercises(context.Background(), userID, programID, uuid.New(), []WeekExerciseReorderDay{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ReorderWeekExercises() error = %v, want ErrNotFound", err)
	}
}

func TestService_AddWeek_forbidden(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: ownerID, Status: StatusDraft},
	}, &fakeRoleQuerier{isAdmin: false})
	_, err := svc.AddWeek(context.Background(), otherID, programID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("AddWeek() error = %v, want ErrForbidden", err)
	}
}

func TestService_ReorderDays_invalidReorder(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&reorderDaysErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: ErrInvalidReorder,
	}, nil)
	_, err := svc.ReorderDays(context.Background(), userID, programID, uuid.New(), []uuid.UUID{uuid.New()})
	if !errors.Is(err, ErrInvalidReorder) {
		t.Fatalf("ReorderDays() error = %v, want ErrInvalidReorder", err)
	}
}

func TestService_ReorderWeekExercises_invalidReorder(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&reorderExercisesErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: ErrInvalidReorder,
	}, nil)
	_, err := svc.ReorderWeekExercises(context.Background(), userID, programID, uuid.New(), nil)
	if !errors.Is(err, ErrInvalidReorder) {
		t.Fatalf("ReorderWeekExercises() error = %v, want ErrInvalidReorder", err)
	}
}

func TestService_DeleteDay_lastDay(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&deleteDayErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		deleteDayErr: ErrLastDay,
	}, nil)
	_, err := svc.DeleteDay(context.Background(), userID, programID, uuid.New(), uuid.New())
	if !errors.Is(err, ErrLastDay) {
		t.Fatalf("DeleteDay() error = %v, want ErrLastDay", err)
	}
}
