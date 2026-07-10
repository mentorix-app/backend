package trainerclient

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/program"
)

type ClientProgramService interface {
	GetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID) (*program.Assignment, error)
	SetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID, programID *uuid.UUID) (*program.Assignment, error)
}

type InviteSettings struct {
	TelegramBotUsername string
	InviteTTL           time.Duration
}

type Service struct {
	store         *Store
	programs      ClientProgramService
	invites       InviteSettings
	activeTrainer ActiveTrainerStore
	notifier      ProgramNotifier
}

type ProgramNotifier interface {
	NotifyProgramAssigned(ctx context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error
}

func NewService(pool *pgxpool.Pool, programs ClientProgramService, invites InviteSettings, activeTrainer ActiveTrainerStore, notifier ProgramNotifier) *Service {
	return &Service{
		store:         NewStore(pool),
		programs:      programs,
		invites:       invites,
		activeTrainer: activeTrainer,
		notifier:      notifier,
	}
}

func (s *Service) CreateInvite(ctx context.Context, trainerUserID uuid.UUID) (Invite, error) {
	if strings.TrimSpace(s.invites.TelegramBotUsername) == "" {
		return Invite{}, ErrInviteNotConfigured
	}
	trainerID, err := s.store.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invite{}, program.ErrForbidden
		}
		return Invite{}, err
	}
	row, token, err := s.store.CreateInvite(ctx, trainerID, s.invites.InviteTTL)
	if err != nil {
		return Invite{}, err
	}
	return Invite{
		ID:        pgconv.FromPGUUID(row.ID),
		InviteURL: inviteURL(s.invites.TelegramBotUsername, token),
		ExpiresAt: row.ExpiresAt.UTC(),
		CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

func (s *Service) ListClients(ctx context.Context, trainerUserID uuid.UUID) (ClientListResult, error) {
	trainerID, err := s.store.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ClientListResult{}, program.ErrForbidden
		}
		return ClientListResult{}, err
	}
	items, err := s.store.ListClients(ctx, trainerID)
	if err != nil {
		return ClientListResult{}, err
	}
	return ClientListResult{Items: items}, nil
}

func (s *Service) AcceptInvite(ctx context.Context, req AcceptInviteRequest) (AcceptInviteResult, error) {
	result, err := s.store.AcceptInvite(ctx, req.Token, req.TelegramUserID, req.DisplayName)
	if err != nil {
		return AcceptInviteResult{}, err
	}
	if s.activeTrainer != nil {
		_ = s.activeTrainer.Set(ctx, req.TelegramUserID, result.TrainerID)
	}
	return result, nil
}

func (s *Service) GetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID) (*program.Assignment, error) {
	return s.programs.GetClientProgramAssignment(ctx, trainerUserID, clientUserID)
}

func (s *Service) SetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID, programID *uuid.UUID) (*program.Assignment, error) {
	assignment, err := s.programs.SetClientProgramAssignment(ctx, trainerUserID, clientUserID, programID)
	if err != nil {
		return nil, err
	}
	if assignment != nil && s.notifier != nil {
		_ = s.notifier.NotifyProgramAssigned(ctx, assignment.ClientUserID, assignment.TrainerID, assignment.ProgramVersionID)
	}
	return assignment, nil
}
