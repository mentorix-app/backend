package exercise

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/subscription"
)

// fakeRoles reports admin membership for the ids in admins.
type fakeRoles struct {
	admins map[uuid.UUID]bool
}

func (f *fakeRoles) UserHasAnyRole(_ context.Context, arg sqlc.UserHasAnyRoleParams) (bool, error) {
	id := uuid.UUID(arg.UserID.Bytes)
	return f.admins[id], nil
}

type stubExerciseStore struct {
	listResult ListResult
	getResult  Exercise
	create     Exercise
	update     Exercise
	listErr    error
	getErr     error
	createErr  error
	updateErr  error
	deleteN    int64
	deleteErr  error
	trainerID  uuid.UUID
	trainerErr error
}

func (s *stubExerciseStore) List(context.Context, Viewer, ListParams) (ListResult, error) {
	return s.listResult, s.listErr
}

func (s *stubExerciseStore) GetByID(context.Context, uuid.UUID) (Exercise, error) {
	return s.getResult, s.getErr
}

func (s *stubExerciseStore) Create(context.Context, uuid.UUID, *uuid.UUID, UpsertInput) (Exercise, error) {
	return s.create, s.createErr
}

func (s *stubExerciseStore) Update(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, UpsertInput) (Exercise, error) {
	return s.update, s.updateErr
}

func (s *stubExerciseStore) DeleteMany(context.Context, uuid.UUID, *uuid.UUID, []uuid.UUID) (int64, error) {
	return s.deleteN, s.deleteErr
}

func (s *stubExerciseStore) TrainerIDForUser(context.Context, uuid.UUID) (uuid.UUID, error) {
	return s.trainerID, s.trainerErr
}

// trainerService wires a Service where the acting user is a plain trainer.
func trainerService(store exerciseStore) *Service {
	return NewServiceWithStore(store, &fakeRoles{admins: map[uuid.UUID]bool{}}, nil)
}

func TestService_Create_validatesInput(t *testing.T) {
	svc := trainerService(&stubExerciseStore{trainerID: uuid.New()})
	_, err := svc.Create(context.Background(), uuid.New(), UpsertInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestService_Create_success(t *testing.T) {
	want := Exercise{ID: uuid.New(), Name: "Squat"}
	svc := trainerService(&stubExerciseStore{create: want, trainerID: uuid.New()})
	got, err := svc.Create(context.Background(), uuid.New(), UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        ExerciseTypeStrength,
		MuscleGroup: MuscleGroupLegs,
		Difficulty:  DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.Name != want.Name {
		t.Errorf("name = %q", got.Name)
	}
}

func TestService_Update_validatesInput(t *testing.T) {
	svc := trainerService(&stubExerciseStore{trainerID: uuid.New()})
	_, err := svc.Update(context.Background(), uuid.New(), uuid.New(), UpsertInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestService_GetAndList(t *testing.T) {
	id := uuid.New()
	svc := trainerService(&stubExerciseStore{
		getResult:  Exercise{ID: id, Scope: ScopeGlobal},
		listResult: ListResult{Items: []Exercise{{ID: id, Scope: ScopeGlobal}}},
		trainerID:  uuid.New(),
	})
	userID := uuid.New()
	got, err := svc.Get(context.Background(), userID, id)
	if err != nil || got.ID != id {
		t.Fatalf("Get() = %v, %v", got, err)
	}
	list, err := svc.List(context.Background(), userID, ListParams{})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("List() = %+v, %v", list, err)
	}
}

func TestService_Get_privateHiddenFromOtherTrainer(t *testing.T) {
	owner := uuid.New()
	viewerTrainer := uuid.New()
	private := Exercise{ID: uuid.New(), Scope: ScopePrivate}
	private.ownerTrainerID = &owner
	svc := trainerService(&stubExerciseStore{getResult: private, trainerID: viewerTrainer})

	_, err := svc.Get(context.Background(), uuid.New(), private.ID)
	if err == nil {
		t.Fatal("expected not-found for foreign private exercise")
	}
}

func TestService_Create_propagatesStoreError(t *testing.T) {
	want := errors.New("db down")
	svc := trainerService(&stubExerciseStore{createErr: want, trainerID: uuid.New()})
	_, err := svc.Create(context.Background(), uuid.New(), UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        ExerciseTypeStrength,
		MuscleGroup: MuscleGroupLegs,
		Difficulty:  DifficultyBeginner,
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestService_Update_success(t *testing.T) {
	want := Exercise{ID: uuid.New(), Name: "Updated"}
	svc := trainerService(&stubExerciseStore{update: want, trainerID: uuid.New()})
	got, err := svc.Update(context.Background(), uuid.New(), uuid.New(), UpsertInput{
		Name:        "Updated",
		NameRu:      "Обновлено",
		Type:        ExerciseTypeStrength,
		MuscleGroup: MuscleGroupLegs,
		Difficulty:  DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.Name != want.Name {
		t.Errorf("name = %q", got.Name)
	}
}

func TestService_DeleteMany_success(t *testing.T) {
	svc := trainerService(&stubExerciseStore{deleteN: 2, trainerID: uuid.New()})
	got, err := svc.DeleteMany(context.Background(), uuid.New(), []uuid.UUID{uuid.New(), uuid.New()})
	if err != nil {
		t.Fatalf("DeleteMany() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
}

func TestService_Get_notFound(t *testing.T) {
	svc := trainerService(&stubExerciseStore{getErr: errors.New("not found"), trainerID: uuid.New()})
	_, err := svc.Get(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_Update_propagatesStoreError(t *testing.T) {
	want := errors.New("db down")
	svc := trainerService(&stubExerciseStore{updateErr: want, trainerID: uuid.New()})
	_, err := svc.Update(context.Background(), uuid.New(), uuid.New(), UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        ExerciseTypeStrength,
		MuscleGroup: MuscleGroupLegs,
		Difficulty:  DifficultyBeginner,
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestService_Update_quotaExceeded(t *testing.T) {
	want := &subscription.QuotaError{Resource: subscription.ResourceExercises, Plan: subscription.PlanFree, Limit: 10, Usage: 11}
	svc := trainerService(&stubExerciseStore{trainerID: uuid.New()})
	svc.quota = &fakeQuota{err: want}
	_, err := svc.Update(context.Background(), uuid.New(), uuid.New(), UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        ExerciseTypeStrength,
		MuscleGroup: MuscleGroupLegs,
		Difficulty:  DifficultyBeginner,
	})
	if !errors.Is(err, want) && err.Error() != want.Error() {
		var qe *subscription.QuotaError
		if !errors.As(err, &qe) {
			t.Fatalf("err = %v, want QuotaError", err)
		}
	}
}

type fakeQuota struct{ err error }

func (f *fakeQuota) CheckQuota(context.Context, uuid.UUID, subscription.Resource, subscription.Op) error {
	return f.err
}

func TestService_AdminCreatesGlobal(t *testing.T) {
	adminID := uuid.New()
	store := &stubExerciseStore{create: Exercise{ID: uuid.New(), Scope: ScopeGlobal}}
	svc := NewServiceWithStore(store, &fakeRoles{admins: map[uuid.UUID]bool{adminID: true}}, nil)

	got, err := svc.Create(context.Background(), adminID, UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        ExerciseTypeStrength,
		MuscleGroup: MuscleGroupLegs,
		Difficulty:  DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.Scope != ScopeGlobal {
		t.Fatalf("scope = %s, want global", got.Scope)
	}
}
