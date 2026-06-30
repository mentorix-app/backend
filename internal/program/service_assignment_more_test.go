package program

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestNewServiceWithStore(t *testing.T) {
	store := &fakeProgramStore{}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})
	if svc == nil {
		t.Fatal("expected service")
	}
}

func TestService_GetClientProgramAssignment_success(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	trainerID := uuid.New()
	want := &Assignment{ID: uuid.New(), ClientUserID: clientID}
	store := &trainerClientStore{
		trainerID:  trainerID,
		assignment: want,
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	got, err := svc.GetClientProgramAssignment(context.Background(), userID, clientID)
	if err != nil {
		t.Fatalf("GetClientProgramAssignment() error = %v", err)
	}
	if got != want {
		t.Fatalf("assignment = %+v, want %+v", got, want)
	}
}

func TestService_GetClientProgramAssignment_noTrainerRow(t *testing.T) {
	store := &trainerClientStore{fakeProgramStore: fakeProgramStore{err: pgx.ErrNoRows}}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	_, err := svc.GetClientProgramAssignment(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
}

func TestService_SetClientProgramAssignment_success(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	programID := uuid.New()
	want := &Assignment{ID: uuid.New(), ProgramID: programID}
	store := &trainerClientStore{setAssignment: want}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	got, err := svc.SetClientProgramAssignment(context.Background(), userID, clientID, &programID)
	if err != nil {
		t.Fatalf("SetClientProgramAssignment() error = %v", err)
	}
	if got != want {
		t.Fatalf("assignment = %+v, want %+v", got, want)
	}
}

type trainerClientStore struct {
	fakeProgramStore
	trainerID        uuid.UUID
	assignment       *Assignment
	assignmentErr    error
	setAssignment    *Assignment
	setAssignmentErr error
}

func (s *trainerClientStore) TrainerIDForUser(context.Context, uuid.UUID) (uuid.UUID, error) {
	if s.err != nil {
		return uuid.Nil, s.err
	}
	if s.trainerID != uuid.Nil {
		return s.trainerID, nil
	}
	return uuid.New(), nil
}

func (s *trainerClientStore) GetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID) (*Assignment, error) {
	return s.assignment, s.assignmentErr
}

func (s *trainerClientStore) SetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *uuid.UUID) (*Assignment, error) {
	return s.setAssignment, s.setAssignmentErr
}

func TestService_GetClientProgramAssignment_nilResult(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	store := &trainerClientStore{
		assignment: nil,
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	got, err := svc.GetClientProgramAssignment(context.Background(), userID, clientID)
	if err != nil {
		t.Fatalf("GetClientProgramAssignment() error = %v", err)
	}
	if got != nil {
		t.Fatalf("assignment = %+v, want nil", got)
	}
}

func TestService_GetClientProgramAssignment_lookupError(t *testing.T) {
	store := &trainerClientStore{
		fakeProgramStore: fakeProgramStore{err: errors.New("db down")},
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	_, err := svc.GetClientProgramAssignment(context.Background(), uuid.New(), uuid.New())
	if err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("GetClientProgramAssignment() error = %v, want generic error", err)
	}
}
