package exercise

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type stubExerciseStore struct {
	listResult ListResult
	getResult  Exercise
	create     Exercise
	update     Exercise
	listErr    error
	getErr     error
	createErr  error
	updateErr  error
}

func (s *stubExerciseStore) List(context.Context, ListParams) (ListResult, error) {
	return s.listResult, s.listErr
}

func (s *stubExerciseStore) GetByID(context.Context, uuid.UUID) (Exercise, error) {
	return s.getResult, s.getErr
}

func (s *stubExerciseStore) Create(context.Context, uuid.UUID, UpsertInput) (Exercise, error) {
	return s.create, s.createErr
}

func (s *stubExerciseStore) Update(context.Context, uuid.UUID, uuid.UUID, UpsertInput) (Exercise, error) {
	return s.update, s.updateErr
}

func (s *stubExerciseStore) DeleteMany(context.Context, uuid.UUID, []uuid.UUID) (int64, error) {
	return 0, nil
}

func TestService_Create_validatesInput(t *testing.T) {
	svc := &Service{store: &stubExerciseStore{}}
	_, err := svc.Create(context.Background(), uuid.New(), UpsertInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestService_Create_success(t *testing.T) {
	want := Exercise{ID: uuid.New(), Name: "Squat"}
	svc := &Service{store: &stubExerciseStore{create: want}}
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
	svc := &Service{store: &stubExerciseStore{}}
	_, err := svc.Update(context.Background(), uuid.New(), uuid.New(), UpsertInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestService_GetAndList(t *testing.T) {
	id := uuid.New()
	svc := &Service{store: &stubExerciseStore{
		getResult:  Exercise{ID: id},
		listResult: ListResult{Items: []Exercise{{ID: id}}},
	}}
	got, err := svc.Get(context.Background(), id)
	if err != nil || got.ID != id {
		t.Fatalf("Get() = %v, %v", got, err)
	}
	list, err := svc.List(context.Background(), ListParams{})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("List() = %+v, %v", list, err)
	}
}

func TestService_Create_propagatesStoreError(t *testing.T) {
	want := errors.New("db down")
	svc := &Service{store: &stubExerciseStore{createErr: want}}
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
	svc := &Service{store: &stubExerciseStore{update: want}}
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

type countingDeleteStore struct {
	stubExerciseStore
	count int64
	err   error
}

func (d *countingDeleteStore) DeleteMany(context.Context, uuid.UUID, []uuid.UUID) (int64, error) {
	return d.count, d.err
}

func TestService_DeleteMany_success(t *testing.T) {
	svc := &Service{store: &countingDeleteStore{count: 2}}
	got, err := svc.DeleteMany(context.Background(), uuid.New(), []uuid.UUID{uuid.New(), uuid.New()})
	if err != nil {
		t.Fatalf("DeleteMany() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
}

func TestService_Get_notFound(t *testing.T) {
	svc := &Service{store: &stubExerciseStore{getErr: errors.New("not found")}}
	_, err := svc.Get(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_Update_propagatesStoreError(t *testing.T) {
	want := errors.New("db down")
	svc := &Service{store: &stubExerciseStore{updateErr: want}}
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
