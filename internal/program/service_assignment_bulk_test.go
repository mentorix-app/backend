package program

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestBulkAssignmentSkipReason_alreadyAssigned(t *testing.T) {
	if bulkAssignmentSkipReason(ErrAlreadyAssigned) != "already_assigned" {
		t.Fatal(bulkAssignmentSkipReason(ErrAlreadyAssigned))
	}
}

func TestBulkSetClientProgramAssignmentRequest_validate(t *testing.T) {
	if err := (BulkSetClientProgramAssignmentRequest{}).Validate(); !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v", err)
	}

	ids := make([]uuid.UUID, MaxBulkClientAssignments+1)
	if err := (BulkSetClientProgramAssignmentRequest{ClientUserIDs: ids}).Validate(); !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v", err)
	}

	if err := (BulkSetClientProgramAssignmentRequest{ClientUserIDs: []uuid.UUID{uuid.New()}}).Validate(); err != nil {
		t.Fatalf("valid request: %v", err)
	}
}

func TestUniqueUUIDs(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got := uniqueUUIDs([]uuid.UUID{a, a, b})
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("got %v", got)
	}
}

func TestBulkAssignmentSkipReason(t *testing.T) {
	if bulkAssignmentSkipReason(ErrClientNotLinked) != "not_linked" {
		t.Fatal("expected not_linked")
	}
	if bulkAssignmentSkipReason(ErrClientBlocked) != "blocked" {
		t.Fatal("expected blocked")
	}
	if bulkAssignmentSkipReason(ErrClientNotFound) != "not_found" {
		t.Fatal("expected not_found")
	}
	if bulkAssignmentSkipReason(errors.New("other")) != "" {
		t.Fatal("expected empty reason")
	}
}

func TestService_BulkSetClientProgramAssignment_partialSuccess(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	linkedClient := uuid.New()
	otherClient := uuid.New()

	store := &bulkTrainerClientStore{
		trainerClientStore: trainerClientStore{trainerID: uuid.New()},
		setFn: func(clientUserID uuid.UUID) (*Assignment, error) {
			if clientUserID == otherClient {
				return nil, ErrClientNotLinked
			}
			return &Assignment{ClientUserID: clientUserID, ProgramID: programID}, nil
		},
		validateProgramErr: nil,
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	result, err := svc.BulkSetClientProgramAssignment(context.Background(), userID, BulkSetClientProgramAssignmentRequest{
		ProgramID:     &programID,
		ClientUserIDs: []uuid.UUID{linkedClient, otherClient},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment() error = %v", err)
	}
	if len(result.Assigned) != 1 || result.Assigned[0].ClientUserID != linkedClient {
		t.Fatalf("assigned = %+v", result.Assigned)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "not_linked" {
		t.Fatalf("skipped = %+v", result.Skipped)
	}
}

func TestService_BulkSetClientProgramAssignment_clear(t *testing.T) {
	userID := uuid.New()
	clientID := uuid.New()
	store := &bulkTrainerClientStore{
		trainerClientStore: trainerClientStore{trainerID: uuid.New()},
		setFn: func(uuid.UUID) (*Assignment, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	result, err := svc.BulkSetClientProgramAssignment(context.Background(), userID, BulkSetClientProgramAssignmentRequest{
		ClientUserIDs: []uuid.UUID{clientID},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment() error = %v", err)
	}
	if len(result.Cleared) != 1 || result.Cleared[0] != clientID {
		t.Fatalf("cleared = %+v", result.Cleared)
	}
}

func TestService_BulkSetClientProgramAssignment_invalidProgram(t *testing.T) {
	store := &bulkTrainerClientStore{
		trainerClientStore: trainerClientStore{trainerID: uuid.New()},
		validateProgramErr: ErrProgramNotPublished,
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	programID := uuid.New()
	_, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), BulkSetClientProgramAssignmentRequest{
		ProgramID:     &programID,
		ClientUserIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrProgramNotPublished) {
		t.Fatalf("error = %v", err)
	}
}

func TestService_BulkSetClientProgramAssignment_skipBlocked(t *testing.T) {
	clientID := uuid.New()
	programID := uuid.New()
	store := &bulkTrainerClientStore{
		trainerClientStore: trainerClientStore{trainerID: uuid.New()},
		setFn: func(uuid.UUID) (*Assignment, error) {
			return nil, ErrClientBlocked
		},
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	result, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), BulkSetClientProgramAssignmentRequest{
		ProgramID:     &programID,
		ClientUserIDs: []uuid.UUID{clientID},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment() error = %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "blocked" {
		t.Fatalf("skipped = %+v", result.Skipped)
	}
}

func TestService_BulkSetClientProgramAssignment_dedupesClientIDs(t *testing.T) {
	clientID := uuid.New()
	programID := uuid.New()
	calls := 0
	store := &bulkTrainerClientStore{
		trainerClientStore: trainerClientStore{trainerID: uuid.New()},
		setFn: func(clientUserID uuid.UUID) (*Assignment, error) {
			calls++
			return &Assignment{ClientUserID: clientUserID, ProgramID: programID}, nil
		},
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	result, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), BulkSetClientProgramAssignmentRequest{
		ProgramID:     &programID,
		ClientUserIDs: []uuid.UUID{clientID, clientID},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment() error = %v", err)
	}
	if calls != 1 || len(result.Assigned) != 1 {
		t.Fatalf("calls=%d assigned=%+v", calls, result.Assigned)
	}
}

func TestService_BulkSetClientProgramAssignment_skipNotFound(t *testing.T) {
	clientID := uuid.New()
	store := &bulkTrainerClientStore{
		trainerClientStore: trainerClientStore{trainerID: uuid.New()},
		setFn: func(uuid.UUID) (*Assignment, error) {
			return nil, ErrClientNotFound
		},
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	result, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), BulkSetClientProgramAssignmentRequest{
		ClientUserIDs: []uuid.UUID{clientID},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment() error = %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != "not_found" {
		t.Fatalf("skipped = %+v", result.Skipped)
	}
}

func TestService_BulkSetClientProgramAssignment_storeError(t *testing.T) {
	store := &bulkTrainerClientStore{
		trainerClientStore: trainerClientStore{trainerID: uuid.New()},
		setFn: func(uuid.UUID) (*Assignment, error) {
			return nil, errors.New("db down")
		},
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	_, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), BulkSetClientProgramAssignmentRequest{
		ClientUserIDs: []uuid.UUID{uuid.New()},
	})
	if err == nil || err.Error() != "db down" {
		t.Fatalf("error = %v", err)
	}
}

func TestService_BulkSetClientProgramAssignment_trainerLookupError(t *testing.T) {
	store := &bulkTrainerClientStore{
		trainerClientStore: trainerClientStore{fakeProgramStore: fakeProgramStore{err: errors.New("db down")}},
	}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	_, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), BulkSetClientProgramAssignmentRequest{
		ClientUserIDs: []uuid.UUID{uuid.New()},
	})
	if err == nil || err.Error() != "db down" {
		t.Fatalf("error = %v", err)
	}
}

func TestService_BulkSetClientProgramAssignment_noTrainer(t *testing.T) {
	store := &trainerClientStore{fakeProgramStore: fakeProgramStore{err: pgx.ErrNoRows}}
	svc := NewServiceWithStore(store, &fakeRoleQuerier{isAdmin: true})

	programID := uuid.New()
	_, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), BulkSetClientProgramAssignmentRequest{
		ProgramID:     &programID,
		ClientUserIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v", err)
	}
}

type bulkTrainerClientStore struct {
	trainerClientStore
	setFn              func(clientUserID uuid.UUID) (*Assignment, error)
	validateProgramErr error
}

func (s *bulkTrainerClientStore) SetClientProgramAssignment(_ context.Context, _, _, clientUserID uuid.UUID, _ *uuid.UUID) (*Assignment, error) {
	if s.setFn != nil {
		return s.setFn(clientUserID)
	}
	return s.setAssignment, s.setAssignmentErr
}

func (s *bulkTrainerClientStore) validateProgramForAssignment(context.Context, uuid.UUID, uuid.UUID) error {
	return s.validateProgramErr
}
