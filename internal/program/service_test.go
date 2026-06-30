package program

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/exercise"
)

type fakeRoleQuerier struct {
	isAdmin bool
	err     error
}

func (f *fakeRoleQuerier) UserHasAnyRole(context.Context, sqlc.UserHasAnyRoleParams) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.isAdmin, nil
}

func testService(store programStore, roles *fakeRoleQuerier) *Service {
	if roles == nil {
		roles = &fakeRoleQuerier{isAdmin: true}
	}
	return &Service{store: store, roles: roles}
}

type fakeProgramStore struct {
	program      Program
	detail       Detail
	listResult   ListResult
	createDetail Detail
	err          error
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

func (f *fakeProgramStore) SetStatus(_ context.Context, _, _ uuid.UUID, status Status) (Detail, error) {
	if f.err != nil {
		return Detail{}, f.err
	}
	out := f.detail
	out.Program.Status = status
	return out, nil
}

func (f *fakeProgramStore) PublishFromDraft(_ context.Context, _, _ uuid.UUID, d Detail) (Detail, error) {
	if f.err != nil {
		return Detail{}, f.err
	}
	out := d
	out.Program.Status = StatusPublished
	out.HasUnpublishedChanges = false
	return out, nil
}

func (f *fakeProgramStore) FreezePublishedVersion(_ context.Context, _, _ uuid.UUID, d Detail) (Detail, error) {
	if f.err != nil {
		return Detail{}, f.err
	}
	out := d
	out.HasUnpublishedChanges = false
	return out, nil
}

func (f *fakeProgramStore) SoftDelete(context.Context, uuid.UUID, uuid.UUID) error {
	return f.err
}

func (f *fakeProgramStore) AddWeek(context.Context, uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) DeleteWeek(context.Context, uuid.UUID, uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) AddDay(context.Context, uuid.UUID, uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) DeleteDay(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) AddDayExercise(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, DayExerciseInput) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) UpdateDayExercise(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, DayExerciseInput) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) DeleteDayExercise(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) ReorderWeeks(context.Context, uuid.UUID, []uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) ReorderDays(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) ReorderWeekExercises(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, []WeekExerciseReorderDay) (Detail, error) {
	return f.detail, f.err
}

func (f *fakeProgramStore) TrainerIDForUser(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), f.err
}

func (f *fakeProgramStore) GetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID) (*Assignment, error) {
	return nil, f.err
}

func (f *fakeProgramStore) SetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *uuid.UUID) (*Assignment, error) {
	return nil, f.err
}

func (f *fakeProgramStore) ListProgramAssignments(context.Context, uuid.UUID) (AssignmentListResult, error) {
	return AssignmentListResult{}, f.err
}

func (f *fakeProgramStore) SyncProgramAssignments(context.Context, uuid.UUID, AssignmentSyncRequest) (AssignmentSyncResult, error) {
	return AssignmentSyncResult{}, f.err
}

func (f *fakeProgramStore) ListProgramVersions(context.Context, uuid.UUID) (VersionListResult, error) {
	return VersionListResult{}, f.err
}

func (f *fakeProgramStore) DeleteProgramVersion(context.Context, uuid.UUID, uuid.UUID) error {
	return f.err
}

func (f *fakeProgramStore) CleanupProgramVersions(context.Context, uuid.UUID) (VersionCleanupResult, error) {
	return VersionCleanupResult{}, f.err
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
	store := &capturingStore{}
	svc := testService(store, &fakeRoleQuerier{isAdmin: false})

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
	svc := testService(&fakeProgramStore{
		program: Program{CreatedBy: ownerID},
	}, &fakeRoleQuerier{isAdmin: false})

	_, err := svc.Get(context.Background(), otherID, uuid.New())
	if err != ErrForbidden {
		t.Fatalf("Get() error = %v, want ErrForbidden", err)
	}
}

func publishableWeek(dayExercises []DayExercise) Week {
	return Week{
		WeekNumber: 1,
		SortOrder:  1,
		Days: []Day{{
			DayNumber: 1,
			SortOrder: 1,
			Exercises: dayExercises,
		}},
	}
}

func TestService_Publish_invalidTransitionFromPublished(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	category := CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	sets := 3
	reps := 10

	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		detail: Detail{
			Program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished, Name: "Program", Category: &category, Difficulty: &difficulty},
			Weeks: []Week{publishableWeek([]DayExercise{{
				ExerciseID: uuid.New(),
				Sets:       &sets,
				Reps:       &reps,
			}})},
		},
	}, nil)

	_, err := svc.Publish(context.Background(), userID, programID)
	if err != ErrInvalidStatusTransition {
		t.Fatalf("Publish() error = %v, want ErrInvalidStatusTransition", err)
	}
}

func TestService_Publish_validationError(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		detail: Detail{
			Program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
			Weeks:   []Week{publishableWeek([]DayExercise{})},
		},
	}, nil)

	_, err := svc.Publish(context.Background(), userID, programID)
	if err == nil || err == ErrInvalidStatusTransition {
		t.Fatalf("Publish() error = %v, want validation error", err)
	}
}

func TestService_Delete_idempotentWhenAlreadyDeleted(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	deleted := time.Now().UTC()
	svc := testService(&fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, DeletedAt: &deleted},
	}, nil)

	if err := svc.Delete(context.Background(), userID, programID); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
}
