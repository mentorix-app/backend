package trainerclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
)

func (s *Store) ClientUserIDByTelegram(ctx context.Context, telegramUserID string) (uuid.UUID, error) {
	userPG, err := s.q.GetClientUserIDByTelegram(ctx, sqlc.GetClientUserIDByTelegramParams{
		Provider: auth.ProviderTelegram,
		Subject:  strings.TrimSpace(telegramUserID),
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return uuid.Nil, ErrTelegramUserNotFound
		}
		return uuid.Nil, fmt.Errorf("get client by telegram: %w", err)
	}
	return pgconv.FromPGUUID(userPG), nil
}

func (s *Store) ListTelegramTrainers(ctx context.Context, telegramUserID string) ([]TelegramTrainer, error) {
	rows, err := s.q.ListClientTrainersByTelegramUser(ctx, sqlc.ListClientTrainersByTelegramUserParams{
		Provider: auth.ProviderTelegram,
		Subject:  strings.TrimSpace(telegramUserID),
	})
	if err != nil {
		return nil, fmt.Errorf("list telegram trainers: %w", err)
	}
	out := make([]TelegramTrainer, 0, len(rows))
	for _, row := range rows {
		trainer := TelegramTrainer{
			TrainerID:   pgconv.FromPGUUID(row.TrainerID),
			DisplayName: row.TrainerDisplayName,
			HasProgram:  row.AssignmentID.Valid,
		}
		if row.ProgramName != nil {
			trainer.ProgramName = *row.ProgramName
		}
		if row.ProgramNameRu != nil {
			trainer.ProgramNameRu = *row.ProgramNameRu
		}
		out = append(out, trainer)
	}
	return out, nil
}

func (s *Store) TrainerLinkedToClient(ctx context.Context, trainerID, clientUserID uuid.UUID) (string, error) {
	row, err := s.q.GetTrainerClient(ctx, sqlc.GetTrainerClientParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrTrainerNotLinked
		}
		return "", fmt.Errorf("get trainer client: %w", err)
	}
	if row.Status == "blocked" {
		return "", program.ErrClientBlocked
	}
	name, err := s.q.GetTrainerUserDisplayName(ctx, pgconv.ToPGUUID(trainerID))
	if err != nil {
		return "", fmt.Errorf("trainer display name: %w", err)
	}
	return name, nil
}

func pickProgramName(name, nameRu string) string {
	if strings.TrimSpace(nameRu) != "" {
		return strings.TrimSpace(nameRu)
	}
	return strings.TrimSpace(name)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
