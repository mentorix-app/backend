package program

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/exercise"
)

type fakeProgramStore struct {
	isAdmin      bool
	program      Program
	detail       Detail
	listResult   ListResult
	createDetail Detail
	err          error
}

func (f *fakeProgramStore) IsAdmin(context.Context, uuid.UUID) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.isAdmin, nil
}

func (f *fakeProgramStore) CreateDraft(context.Context, uuid.UUID) (Detail, error) {
	return f.createDetail, f.err
}

func (f *fakeProgramStore) List(context.Context, ListParams) (ListResult, error) {
	return f.listResult, f.err
}

func (f *fakeProgramStore) GetProgramRow(context.Context, uuid.UUID) (Program, error) {
	if f.err != nil {
		return Program{}, f.err
	}
	return f.program, nil
}

func (f *fakeProgramStore) GetDetail(context.Context, uuid.UUID) (Detail, error) {
	if f.err != nil {
		return Detail{}, f.err
	}
	return f.detail, nil
}

func (f *fakeProgramStore) Update(context.Context, uuid.UUID, uuid.UUID, UpdateInput) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) SetStatus(context.Context, uuid.UUID, uuid.UUID, Status) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error {
	return f.err
}

func (f *fakeProgramStore) AddDay(context.Context, uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) DeleteDay(context.Context, uuid.UUID, uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) AddDayExercise(context.Context, uuid.UUID, uuid.UUID, DayExerciseInput) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) UpdateDayExercise(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, DayExerciseInput) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) DeleteDayExercise(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

type capturingStore struct {
	fakeProgramStore
	lastParams ListParams
}

func (c *capturingStore) List(_ context.Context, params ListParams) (ListResult, error) {
	c.lastParams = params
	return ListResult{Items: []Program{}}, nil
}

func TestService_List_setsCreatedByForTrainer(t *testing.T) {
	trainerID := uuid.New()
	store := &capturingStore{fakeProgramStore: fakeProgramStore{isAdmin: false}}
	svc := &Service{store: store}

	if _, err := svc.List(context.Background(), trainerID, ListParams{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if store.lastParams.CreatedBy == nil || *store.lastParams.CreatedBy != trainerID {
		t.Fatalf("CreatedBy = %v, want %v", store.lastParams.CreatedBy, trainerID)
	}
}

func TestService_Get_forbiddenForOtherTrainer(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	svc := &Service{store: &fakeProgramStore{
		isAdmin: false,
		program: Program{CreatedBy: ownerID},
	}}

	_, err := svc.Get(context.Background(), otherID, uuid.New())
	if err != ErrForbidden {
		t.Fatalf("Get() error = %v, want ErrForbidden", err)
	}
}

func TestService_Publish_invalidTransitionFromPublished(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10

	svc := &Service{store: &fakeProgramStore{
		isAdmin: false,
		program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		detail: Detail{
			Program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished, Name: "Program", Category: &category, Difficulty: &difficulty},
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

	_, err := svc.Publish(context.Background(), userID, programID)
	if err != ErrInvalidStatusTransition {
		t.Fatalf("Publish() error = %v, want ErrInvalidStatusTransition", err)
	}
}

func TestService_Publish_validationError(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail: Detail{
			Program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
			Days:    []Day{{DayNumber: 1, Exercises: []DayExercise{}}},
		},
	}}

	_, err := svc.Publish(context.Background(), userID, programID)
	if err == nil || err == ErrInvalidStatusTransition {
		t.Fatalf("Publish() error = %v, want validation error", err)
	}
}

func TestService_Delete_idempotentWhenAlreadyDeleted(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	deleted := time.Now().UTC()
	svc := &Service{store: &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, DeletedAt: &deleted},
	}}

	if err := svc.Delete(context.Background(), userID, programID); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
}
