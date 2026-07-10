package trainerclient_test

import (
	"context"
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
}

func (s stubAssignPrograms) GetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID) (*program.Assignment, error) {
	return nil, nil
}

func (s stubAssignPrograms) SetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (*program.Assignment, error) {
	return s.assignment, nil
}

func TestService_SetClientProgramAssignment_notifiesOnAssign(t *testing.T) {
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
	_, err := svc.SetClientProgramAssignment(context.Background(), uuid.New(), clientID, &programID)
	if err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}
	if notifier.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", notifier.calls)
	}
	if notifier.last.clientUserID != clientID || notifier.last.trainerID != trainerID || notifier.last.programVersionID != versionID {
		t.Fatalf("unexpected notifier args: %+v", notifier.last)
	}
}

func TestService_SetClientProgramAssignment_skipsNotifyOnClear(t *testing.T) {
	notifier := &recordingNotifier{}
	svc := trainerclient.NewService(nil, stubAssignPrograms{assignment: nil}, trainerclient.InviteSettings{
		InviteTTL: 7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), notifier)

	_, err := svc.SetClientProgramAssignment(context.Background(), uuid.New(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("SetClientProgramAssignment: %v", err)
	}
	if notifier.calls != 0 {
		t.Fatalf("notifier calls = %d, want 0", notifier.calls)
	}
}
