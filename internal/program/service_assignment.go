package program

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) GetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID) (*Assignment, error) {
	trainerID, err := s.store.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	return s.store.GetClientProgramAssignment(ctx, trainerID, clientUserID)
}

func (s *Service) SetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID, programID *uuid.UUID) (*Assignment, error) {
	trainerID, err := s.store.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	return s.store.SetClientProgramAssignment(ctx, trainerUserID, trainerID, clientUserID, programID)
}
