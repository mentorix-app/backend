package workoutcompletion

import (
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
)

type Completion struct {
	ID                  uuid.UUID  `json:"id"`
	ClientUserID        uuid.UUID  `json:"client_user_id"`
	TrainerID           uuid.UUID  `json:"trainer_id"`
	CompletedAt         time.Time  `json:"completed_at"`
	ProgramID           *uuid.UUID `json:"program_id,omitempty"`
	ProgramVersionID    *uuid.UUID `json:"program_version_id,omitempty"`
	ProgramAssignmentID *uuid.UUID `json:"program_assignment_id,omitempty"`
	CompletionCycleID   uuid.UUID  `json:"completion_cycle_id"`
	DayKey              uuid.UUID  `json:"day_key"`
	WeekNumber          int        `json:"week_number"`
	DayNumber           int        `json:"day_number"`
	ProgramName         string     `json:"program_name"`
	ProgramNameRu       string     `json:"program_name_ru"`
	DaySnapshot         []byte     `json:"day_snapshot"`
	ResultText          string     `json:"result_text"`
	Source              string     `json:"source"`
	CreatedAt           time.Time  `json:"created_at"`
}

type CompleteInput struct {
	ClientUserID        uuid.UUID
	TrainerID           uuid.UUID
	ProgramID           uuid.UUID
	ProgramVersionID    uuid.UUID
	ProgramAssignmentID uuid.UUID
	CompletionCycleID   uuid.UUID
	DayKey              uuid.UUID
	WeekNumber          int
	DayNumber           int
	ProgramName         string
	ProgramNameRu       string
	Day                 program.Day
	ResultText          string
	Source              string
}
