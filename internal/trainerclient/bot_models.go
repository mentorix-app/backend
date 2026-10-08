package trainerclient

import (
	"github.com/google/uuid"

	"mentorix-backend/internal/program"
)

type TelegramTrainer struct {
	TrainerID     uuid.UUID `json:"trainer_id"`
	DisplayName   string    `json:"display_name"`
	HasProgram    bool      `json:"has_program"`
	ProgramName   string    `json:"program_name,omitempty"`
	ProgramNameRu string    `json:"program_name_ru,omitempty"`
	IsActive      bool      `json:"is_active"`
}

type TelegramTrainerList struct {
	Items           []TelegramTrainer `json:"items"`
	ActiveTrainerID *uuid.UUID        `json:"active_trainer_id,omitempty"`
}

type SetActiveTrainerRequest struct {
	TelegramUserID string    `json:"telegram_user_id"`
	TrainerID      uuid.UUID `json:"trainer_id"`
}

type ActiveTrainerResponse struct {
	TrainerID   uuid.UUID `json:"trainer_id"`
	DisplayName string    `json:"display_name"`
}

type TelegramProgramResponse struct {
	TrainerID          uuid.UUID             `json:"trainer_id"`
	TrainerDisplayName string                `json:"trainer_display_name"`
	HasProgram         bool                  `json:"has_program"`
	Assignment         *ClientProgramSummary `json:"assignment,omitempty"`
	Program            *program.Detail       `json:"program,omitempty"`
}
