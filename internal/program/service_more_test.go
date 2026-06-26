package program

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/exercise"
)

func TestService_Create(t *testing.T) {
	userID := uuid.New()
	want := Detail{Program: Program{ID: uuid.New(), Status: StatusDraft}}
	svc := &Service{store: &fakeProgramStore{createDetail: want}}
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
	svc := &Service{store: &fakeProgramStore{
		isAdmin: false,
		program: Program{ID: programID, CreatedBy: ownerID, Status: StatusDraft},
	}}
	_, err := svc.Update(context.Background(), otherID, programID, UpdateInput{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Update() error = %v, want ErrForbidden", err)
	}
}

func TestService_Archive_fromPublished(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		detail:  Detail{Program: Program{ID: programID, Status: StatusArchived}},
	}}
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
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  want,
	}}
	got, err := svc.AddDay(context.Background(), userID, programID)
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
	svc := &Service{store: &fakeProgramStore{
		isAdmin: true,
		program: Program{ID: programID, CreatedBy: ownerID},
		detail:  Detail{Program: Program{ID: programID, CreatedBy: ownerID}},
	}}
	got, err := svc.Get(context.Background(), adminID, programID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != programID {
		t.Errorf("id = %v", got.ID)
	}
}

func TestService_Delete_softDeletesDraft(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}}
	if err := svc.Delete(context.Background(), userID, programID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestService_Publish_success(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail: Detail{
			Program: Program{
				ID: programID, CreatedBy: userID, Status: StatusDraft,
				Name: "Program", Category: &category, Difficulty: &difficulty,
			},
			Days: []Day{{
				DayNumber: 1,
				Exercises: []DayExercise{{
					ExerciseID: uuid.New(),
					Sets:       &sets,
					Reps:       &reps,
				}},
			}},
		},
	}}
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
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		detail:  Detail{Program: Program{ID: programID, Status: StatusArchived}},
	}}
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
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  Detail{Program: Program{ID: programID, Name: name}},
	}}
	got, err := svc.Update(context.Background(), userID, programID, UpdateInput{Name: &name})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.Name != name {
		t.Errorf("name = %q", got.Name)
	}
}

func TestService_Get_notFound(t *testing.T) {
	svc := &Service{store: &fakeProgramStore{err: pgx.ErrNoRows}}
	_, err := svc.Get(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestService_Delete_notFound(t *testing.T) {
	svc := &Service{store: &fakeProgramStore{err: pgx.ErrNoRows}}
	err := svc.Delete(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestService_Publish_notFound(t *testing.T) {
	svc := &Service{store: &fakeProgramStore{err: pgx.ErrNoRows}}
	_, err := svc.Publish(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Publish() error = %v, want ErrNotFound", err)
	}
}

func TestService_Archive_invalidTransition(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}}
	_, err := svc.Archive(context.Background(), userID, programID)
	if !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("Archive() error = %v, want ErrInvalidStatusTransition", err)
	}
}

func TestService_dayExerciseOperations(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	dayID := uuid.New()
	itemID := uuid.New()
	sets, reps := 3, 10
	detail := Detail{Program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft}}
	store := &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail:  detail,
	}
	svc := &Service{store: store}
	in := DayExerciseInput{ExerciseID: uuid.New(), Sets: &sets, Reps: &reps}

	if _, err := svc.AddDayExercise(context.Background(), userID, programID, dayID, in); err != nil {
		t.Fatalf("AddDayExercise() error = %v", err)
	}
	if _, err := svc.UpdateDayExercise(context.Background(), userID, programID, dayID, itemID, in); err != nil {
		t.Fatalf("UpdateDayExercise() error = %v", err)
	}
	if _, err := svc.DeleteDayExercise(context.Background(), userID, programID, dayID, itemID); err != nil {
		t.Fatalf("DeleteDayExercise() error = %v", err)
	}
	if _, err := svc.DeleteDay(context.Background(), userID, programID, dayID); err != nil {
		t.Fatalf("DeleteDay() error = %v", err)
	}
}

func TestService_AddDayExercise_validationError(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}}
	_, err := svc.AddDayExercise(context.Background(), userID, programID, uuid.New(), DayExerciseInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

type deleteDayErrStore struct {
	fakeProgramStore
	deleteDayErr error
}

func (d *deleteDayErrStore) DeleteDay(context.Context, uuid.UUID, uuid.UUID) (Detail, error) {
	return Detail{}, d.deleteDayErr
}

func TestService_DeleteDay_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := &Service{store: &deleteDayErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		deleteDayErr: pgx.ErrNoRows,
	}}
	_, err := svc.DeleteDay(context.Background(), userID, programID, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteDay() error = %v, want ErrNotFound", err)
	}
}
