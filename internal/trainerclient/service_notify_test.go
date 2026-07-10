package trainerclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

type recordingNotifier struct {
	calls int
	last  struct {
		clientUserID     uuid.UUID
		trainerID        uuid.UUID
		programVersionID uuid.UUID
	}
}

func (r *recordingNotifier) NotifyProgramAssigned(_ context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error {
	r.calls++
	r.last.clientUserID = clientUserID
	r.last.trainerID = trainerID
	r.last.programVersionID = programVersionID
	return nil
}

type stubAssignPrograms struct {
	assignment *program.Assignment
	bulkErr    error
}

func (s stubAssignPrograms) GetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID) (*program.Assignment, error) {
	return nil, nil
}

func (s stubAssignPrograms) BulkSetClientProgramAssignment(context.Context, uuid.UUID, program.BulkSetClientProgramAssignmentRequest) (program.BulkAssignmentResult, error) {
	if s.bulkErr != nil {
		return program.BulkAssignmentResult{}, s.bulkErr
	}
	if s.assignment == nil {
		return program.BulkAssignmentResult{}, nil
	}
	return program.BulkAssignmentResult{Assigned: []program.Assignment{*s.assignment}}, nil
}

func TestService_BulkSetClientProgramAssignment_propagatesError(t *testing.T) {
	notifier := &recordingNotifier{}
	svc := trainerclient.NewService(nil, stubAssignPrograms{bulkErr: program.ErrValidation}, trainerclient.InviteSettings{
		InviteTTL: 7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), notifier)

	_, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), program.BulkSetClientProgramAssignmentRequest{
		ClientUserIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, program.ErrValidation) {
		t.Fatalf("error = %v", err)
	}
}

func TestService_BulkSetClientProgramAssignment_notifiesOnAssign(t *testing.T) {
	clientID := uuid.New()
	trainerID := uuid.New()
	versionID := uuid.New()
	notifier := &recordingNotifier{}
	assignment := &program.Assignment{
		ClientUserID:     clientID,
		TrainerID:        trainerID,
		ProgramVersionID: versionID,
	}
	svc := trainerclient.NewService(nil, stubAssignPrograms{assignment: assignment}, trainerclient.InviteSettings{
		InviteTTL: 7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), notifier)

	programID := uuid.New()
	_, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), program.BulkSetClientProgramAssignmentRequest{
		ProgramID:     &programID,
		ClientUserIDs: []uuid.UUID{clientID},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment: %v", err)
	}
	if notifier.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", notifier.calls)
	}
	if notifier.last.clientUserID != clientID || notifier.last.trainerID != trainerID || notifier.last.programVersionID != versionID {
		t.Fatalf("unexpected notifier args: %+v", notifier.last)
	}
}

func TestService_BulkSetClientProgramAssignment_skipsNotifyOnClear(t *testing.T) {
	notifier := &recordingNotifier{}
	svc := trainerclient.NewService(nil, stubAssignPrograms{assignment: nil}, trainerclient.InviteSettings{
		InviteTTL: 7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), notifier)

	_, err := svc.BulkSetClientProgramAssignment(context.Background(), uuid.New(), program.BulkSetClientProgramAssignmentRequest{
		ClientUserIDs: []uuid.UUID{uuid.New()},
	})
	if err != nil {
		t.Fatalf("BulkSetClientProgramAssignment: %v", err)
	}
	if notifier.calls != 0 {
		t.Fatalf("notifier calls = %d, want 0", notifier.calls)
	}
}
