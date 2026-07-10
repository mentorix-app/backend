package trainerclient

import (
	"time"

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

type TelegramTodayResponse struct {
	TrainerID          uuid.UUID          `json:"trainer_id"`
	TrainerDisplayName string             `json:"trainer_display_name"`
	HasProgram         bool               `json:"has_program"`
	ProgramName        string             `json:"program_name,omitempty"`
	ProgramNameRu      string             `json:"program_name_ru,omitempty"`
	ProgramDayNumber   int                `json:"program_day_number,omitempty"`
	WeekNumber         int                `json:"week_number,omitempty"`
	DayNumber          int                `json:"day_number,omitempty"`
	IsRestDay          bool               `json:"is_rest_day,omitempty"`
	Blocks             []program.DayBlock `json:"blocks,omitempty"`
	AssignedAt         *time.Time         `json:"assigned_at,omitempty"`
}
