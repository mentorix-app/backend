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

type noopPrograms struct{}

func (noopPrograms) GetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID) (*program.Assignment, error) {
	return nil, nil
}

func (noopPrograms) SetClientProgramAssignment(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (*program.Assignment, error) {
	return nil, nil
}

func TestService_CreateInvite_notConfigured(t *testing.T) {
	svc := trainerclient.NewService(nil, noopPrograms{}, trainerclient.InviteSettings{
		InviteTTL: 7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)
	_, err := svc.CreateInvite(context.Background(), uuid.New())
	if !errors.Is(err, trainerclient.ErrInviteNotConfigured) {
		t.Fatalf("error = %v, want ErrInviteNotConfigured", err)
	}
}
