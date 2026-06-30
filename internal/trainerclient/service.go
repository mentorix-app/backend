package trainerclient

import (
	"context"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
)

type ClientProgramService interface {
	GetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID) (*program.Assignment, error)
	SetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID, programID *uuid.UUID) (*program.Assignment, error)
}
