package trainerclient

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
)

type ClientProgramService interface {
	GetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID) (*program.Assignment, error)
	BulkSetClientProgramAssignment(ctx context.Context, trainerUserID uuid.UUID, req program.BulkSetClientProgramAssignmentRequest) (program.BulkAssignmentResult, error)
}

type ProfilePhotoFetcher interface {
	ProfilePhotoFilePath(ctx context.Context, telegramUserID string) (filePath string, ok bool, err error)
}

type InviteSettings struct {
	TelegramBotUsername string
	InviteTTL           time.Duration
}

type Service struct {
	store         *Store
	programs      ClientProgramService
	roles         auth.RoleQuerier
	invites       InviteSettings
	activeTrainer ActiveTrainerStore
	notifier      ProgramNotifier
	quota         *subscription.Checker
	jwtSecret     string
	botToken      string
	photos        ProfilePhotoFetcher
}

type ServiceOption func(*Service)

func WithAvatarSupport(jwtSecret, botToken string, photos ProfilePhotoFetcher) ServiceOption {
	return func(s *Service) {
		s.jwtSecret = jwtSecret
		s.botToken = botToken
		s.photos = photos
	}
}

type ProgramNotifier interface {
	NotifyProgramAssigned(ctx context.Context, clientUserID, trainerID, programVersionID uuid.UUID) error
}

func NewService(pool *pgxpool.Pool, programs ClientProgramService, invites InviteSettings, activeTrainer ActiveTrainerStore, notifier ProgramNotifier, opts ...ServiceOption) *Service {
	s := &Service{
		store:         NewStore(pool),
		programs:      programs,
		roles:         sqlc.New(pool),
		invites:       invites,
		activeTrainer: activeTrainer,
		notifier:      notifier,
		quota:         subscription.NewChecker(pool),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	// No new invites while the client quota is full; accept re-checks under lock.
	if s.quota != nil {
		if err := s.quota.CheckQuota(ctx, trainerUserID, subscription.ResourceClients, subscription.OpCreate); err != nil {
			return Invite{}, err
		}
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

func (s *Service) ListClients(ctx context.Context, userID uuid.UUID, params ListParams) (ClientListResult, error) {
	isAdmin, err := auth.UserIsAdmin(ctx, s.roles, userID)
	if err != nil {
		return ClientListResult{}, err
	}

	var result ClientListResult
	if isAdmin {
		var preferTrainerID *uuid.UUID
		trainerID, lookupErr := s.store.TrainerIDForUser(ctx, userID)
		if lookupErr == nil {
			preferTrainerID = &trainerID
		} else if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return ClientListResult{}, lookupErr
		}
		result, err = s.store.ListAllClients(ctx, params, preferTrainerID)
	} else {
		var trainerID uuid.UUID
		trainerID, err = s.store.TrainerIDForUser(ctx, userID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ClientListResult{}, program.ErrForbidden
			}
			return ClientListResult{}, err
		}
		result, err = s.store.ListClients(ctx, trainerID, params)
	}
	if err != nil {
		return ClientListResult{}, err
	}
	for i := range result.Items {
		result.Items[i].AvatarURL = BuildAvatarURL(result.Items[i].ClientUserID, result.Items[i].avatarFilePath, s.jwtSecret)
	}
	return result, nil
}

func (s *Service) AcceptInvite(ctx context.Context, req AcceptInviteRequest) (AcceptInviteResult, error) {
	result, err := s.store.AcceptInvite(ctx, req.Token, req.TelegramUserID, req.DisplayName)
	if err != nil {
		return AcceptInviteResult{}, err
	}
	if s.activeTrainer != nil {
		_ = s.activeTrainer.Set(ctx, req.TelegramUserID, result.TrainerID)
	}
	_ = s.syncTelegramAvatar(ctx, req.TelegramUserID, result.UserID)
	return result, nil
}

func (s *Service) RefreshTelegramAvatar(ctx context.Context, telegramUserID string) error {
	userID, err := s.store.UserIDByTelegram(ctx, telegramUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	return s.syncTelegramAvatar(ctx, telegramUserID, userID)
}

func (s *Service) ClientAvatarFilePath(ctx context.Context, clientUserID uuid.UUID) (string, error) {
	return s.store.UserAvatarFilePath(ctx, clientUserID)
}

func (s *Service) BotToken() string {
	return s.botToken
}

func (s *Service) syncTelegramAvatar(ctx context.Context, telegramUserID string, userID uuid.UUID) error {
	if s.photos == nil {
		return nil
	}
	filePath, ok, err := s.photos.ProfilePhotoFilePath(ctx, telegramUserID)
	if err != nil || !ok {
		return err
	}
	return s.store.UpdateUserAvatarFilePath(ctx, userID, filePath)
}

func (s *Service) GetClientProgramAssignment(ctx context.Context, trainerUserID, clientUserID uuid.UUID) (*program.Assignment, error) {
	return s.programs.GetClientProgramAssignment(ctx, trainerUserID, clientUserID)
}

func (s *Service) BulkSetClientProgramAssignment(ctx context.Context, trainerUserID uuid.UUID, req program.BulkSetClientProgramAssignmentRequest) (program.BulkAssignmentResult, error) {
	result, err := s.programs.BulkSetClientProgramAssignment(ctx, trainerUserID, req)
	if err != nil {
		return program.BulkAssignmentResult{}, err
	}
	if s.notifier != nil {
		for _, assignment := range result.Assigned {
			_ = s.notifier.NotifyProgramAssigned(ctx, assignment.ClientUserID, assignment.TrainerID, assignment.ProgramVersionID)
		}
	}
	return result, nil
}
