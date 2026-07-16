package trainerclient

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
)

type ClientProgramReader interface {
	GetAssignmentByTrainerID(ctx context.Context, trainerID, clientUserID uuid.UUID) (*program.Assignment, error)
	GetVersionDetail(ctx context.Context, versionID uuid.UUID) (program.Detail, error)
}

func (s *Service) ListTelegramTrainers(ctx context.Context, telegramUserID string) (TelegramTrainerList, error) {
	trainers, err := s.store.ListTelegramTrainers(ctx, telegramUserID)
	if err != nil {
		if errors.Is(err, ErrTelegramUserNotFound) {
			return TelegramTrainerList{Items: []TelegramTrainer{}}, nil
		}
		return TelegramTrainerList{}, err
	}

	activeID, err := s.resolveActiveTrainerID(ctx, telegramUserID, trainers)
	if err != nil {
		// Still return the list so the client can pick an active trainer in the bot.
		if !errors.Is(err, ErrActiveTrainerNotSet) {
			return TelegramTrainerList{}, err
		}
		activeID = nil
	}

	for i := range trainers {
		trainers[i].IsActive = activeID != nil && trainers[i].TrainerID == *activeID
	}

	return TelegramTrainerList{Items: trainers, ActiveTrainerID: activeID}, nil
}

func (s *Service) GetTelegramActiveTrainer(ctx context.Context, telegramUserID string) (*ActiveTrainerResponse, error) {
	list, err := s.ListTelegramTrainers(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	if list.ActiveTrainerID == nil {
		return nil, ErrActiveTrainerNotSet
	}
	for _, t := range list.Items {
		if t.TrainerID == *list.ActiveTrainerID {
			return &ActiveTrainerResponse{
				TrainerID:   t.TrainerID,
				DisplayName: t.DisplayName,
			}, nil
		}
	}
	return nil, ErrActiveTrainerNotSet
}

func (s *Service) SetTelegramActiveTrainer(ctx context.Context, telegramUserID string, trainerID uuid.UUID) (*ActiveTrainerResponse, error) {
	clientUserID, err := s.store.ClientUserIDByTelegram(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	name, err := s.store.TrainerLinkedToClient(ctx, trainerID, clientUserID)
	if err != nil {
		return nil, err
	}
	if s.activeTrainer != nil {
		if err := s.activeTrainer.Set(ctx, telegramUserID, trainerID); err != nil {
			return nil, err
		}
	}
	return &ActiveTrainerResponse{TrainerID: trainerID, DisplayName: name}, nil
}

func (s *Service) ClientUserIDByTelegram(ctx context.Context, telegramUserID string) (uuid.UUID, error) {
	return s.store.ClientUserIDByTelegram(ctx, telegramUserID)
}

func (s *Service) GetTelegramProgram(ctx context.Context, telegramUserID string, trainerID *uuid.UUID) (TelegramProgramResponse, error) {
	trainer, name, assignment, detail, err := s.clientProgramView(ctx, telegramUserID, trainerID)
	if err != nil {
		return TelegramProgramResponse{}, err
	}
	resp := TelegramProgramResponse{
		TrainerID:          trainer,
		TrainerDisplayName: name,
		HasProgram:         assignment != nil,
	}
	if assignment != nil {
		resp.Assignment = &ClientProgramSummary{
			AssignmentID:      assignment.ID,
			ProgramID:         assignment.ProgramID,
			ProgramVersionID:  assignment.ProgramVersionID,
			AssignedAt:        assignment.AssignedAt,
			CompletionCycleID: assignment.CompletionCycleID,
		}
		resp.Program = &detail
	}
	return resp, nil
}

func (s *Service) GetTelegramToday(ctx context.Context, telegramUserID string, trainerID *uuid.UUID) (TelegramTodayResponse, error) {
	trainer, name, assignment, detail, err := s.clientProgramView(ctx, telegramUserID, trainerID)
	if err != nil {
		return TelegramTodayResponse{}, err
	}
	resp := TelegramTodayResponse{
		TrainerID:          trainer,
		TrainerDisplayName: name,
		HasProgram:         assignment != nil,
	}
	if assignment == nil {
		return resp, nil
	}

	resp.ProgramName = detail.Name
	resp.ProgramNameRu = detail.NameRu
	assignedAt := assignment.AssignedAt.UTC()
	resp.AssignedAt = &assignedAt

	flatDay, programDayNumber, ok := program.TodayFlatDay(assignment.AssignedAt, nowUTC(), detail.Weeks)
	if !ok {
		return resp, nil
	}
	resp.ProgramDayNumber = programDayNumber
	resp.WeekNumber = flatDay.WeekNumber
	resp.DayNumber = flatDay.DayNumber
	resp.IsRestDay = program.IsRestDay(flatDay.Day)
	resp.Blocks = flatDay.Day.Blocks
	return resp, nil
}

func (s *Service) clientProgramView(ctx context.Context, telegramUserID string, trainerID *uuid.UUID) (uuid.UUID, string, *program.Assignment, program.Detail, error) {
	clientUserID, err := s.store.ClientUserIDByTelegram(ctx, telegramUserID)
	if err != nil {
		return uuid.Nil, "", nil, program.Detail{}, err
	}

	resolvedTrainerID, err := s.resolveTrainerForView(ctx, telegramUserID, trainerID)
	if err != nil {
		return uuid.Nil, "", nil, program.Detail{}, err
	}

	name, err := s.store.TrainerLinkedToClient(ctx, resolvedTrainerID, clientUserID)
	if err != nil {
		return uuid.Nil, "", nil, program.Detail{}, err
	}

	reader, ok := s.programs.(ClientProgramReader)
	if !ok {
		return uuid.Nil, "", nil, program.Detail{}, errors.New("program reader not configured")
	}

	assignment, err := reader.GetAssignmentByTrainerID(ctx, resolvedTrainerID, clientUserID)
	if err != nil {
		return uuid.Nil, "", nil, program.Detail{}, err
	}
	if assignment == nil {
		return resolvedTrainerID, name, nil, program.Detail{}, nil
	}

	detail, err := reader.GetVersionDetail(ctx, assignment.ProgramVersionID)
	if err != nil {
		return uuid.Nil, "", nil, program.Detail{}, err
	}
	return resolvedTrainerID, name, assignment, detail, nil
}

func (s *Service) resolveTrainerForView(ctx context.Context, telegramUserID string, trainerID *uuid.UUID) (uuid.UUID, error) {
	if trainerID != nil {
		return *trainerID, nil
	}
	list, err := s.store.ListTelegramTrainers(ctx, telegramUserID)
	if err != nil {
		return uuid.Nil, err
	}
	activeID, err := s.resolveActiveTrainerID(ctx, telegramUserID, list)
	if err != nil {
		return uuid.Nil, err
	}
	if activeID == nil {
		return uuid.Nil, ErrActiveTrainerNotSet
	}
	return *activeID, nil
}

func (s *Service) resolveActiveTrainerID(ctx context.Context, telegramUserID string, trainers []TelegramTrainer) (*uuid.UUID, error) {
	if len(trainers) == 0 {
		return nil, nil
	}
	if len(trainers) == 1 {
		id := trainers[0].TrainerID
		return &id, nil
	}
	if s.activeTrainer == nil {
		return nil, ErrActiveTrainerNotSet
	}
	id, ok, err := s.activeTrainer.Get(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrActiveTrainerNotSet
	}
	for _, t := range trainers {
		if t.TrainerID == id {
			return &id, nil
		}
	}
	if s.activeTrainer != nil {
		_ = s.activeTrainer.Delete(ctx, telegramUserID)
	}
	return nil, ErrActiveTrainerNotSet
}
