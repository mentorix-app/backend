package analytics

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrForbidden       = errors.New("forbidden")
	ErrClientNotFound  = errors.New("client not found")
	ErrProgramNotFound = errors.New("program not found")
)

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// ClientAnalytics is the summary payload for the per-client analytics screen.
type ClientAnalytics struct {
	Client            ClientInfo           `json:"client"`
	CurrentAssignment *AssignmentAnalytics `json:"current_assignment"`
	Activity          ActivityStats        `json:"activity"`
}

type ClientInfo struct {
	ClientUserID uuid.UUID  `json:"client_user_id"`
	DisplayName  string     `json:"display_name"`
	AvatarURL    string     `json:"avatar_url"`
	Status       string     `json:"status"`
	LinkedAt     time.Time  `json:"linked_at"`
	LastActiveAt *time.Time `json:"last_active_at"`
}

type AssignmentAnalytics struct {
	ProgramID        uuid.UUID `json:"program_id"`
	ProgramVersionID uuid.UUID `json:"program_version_id"`
	ProgramName      string    `json:"program_name"`
	ProgramNameRu    string    `json:"program_name_ru"`
	AssignedAt       time.Time `json:"assigned_at"`
	IsBehindLatest   bool      `json:"is_behind_latest"`
	Progress         Progress  `json:"progress"`
}

type Progress struct {
	CompletedDays     int            `json:"completed_days"`
	TotalTrainingDays int            `json:"total_training_days"`
	CompletionPercent float64        `json:"completion_percent"`
	Weeks             []WeekProgress `json:"weeks"`
}

type WeekProgress struct {
	WeekNumber    int `json:"week_number"`
	TotalDays     int `json:"total_days"`
	CompletedDays int `json:"completed_days"`
}

type ActivityStats struct {
	TotalCompletions      int               `json:"total_completions"`
	CompletionsLast7Days  int               `json:"completions_last_7_days"`
	CompletionsLast30Days int               `json:"completions_last_30_days"`
	FirstCompletedAt      *time.Time        `json:"first_completed_at"`
	LastCompletedAt       *time.Time        `json:"last_completed_at"`
	WeekStreak            int               `json:"week_streak"`
	ByProgram             []ProgramActivity `json:"by_program"`
}

// ProgramActivity is a lifetime per-program total across all cycles and versions.
// ProgramID is null when the program row was deleted (journal keeps the name).
type ProgramActivity struct {
	ProgramID        *uuid.UUID `json:"program_id"`
	ProgramName      string     `json:"program_name"`
	ProgramNameRu    string     `json:"program_name_ru"`
	TotalCompletions int        `json:"total_completions"`
	FirstCompletedAt time.Time  `json:"first_completed_at"`
	LastCompletedAt  time.Time  `json:"last_completed_at"`
}

type CompletionsResult struct {
	Items      []CompletionItem `json:"items"`
	Pagination Pagination       `json:"pagination"`
}

type CompletionItem struct {
	ID             uuid.UUID  `json:"id"`
	CompletedAt    time.Time  `json:"completed_at"`
	ProgramID      *uuid.UUID `json:"program_id"`
	ProgramName    string     `json:"program_name"`
	ProgramNameRu  string     `json:"program_name_ru"`
	WeekNumber     int        `json:"week_number"`
	DayNumber      int        `json:"day_number"`
	ResultText     string     `json:"result_text"`
	IsCurrentCycle bool       `json:"is_current_cycle"`
}

type ProgramsAnalyticsResult struct {
	Items      []ProgramAnalyticsItem `json:"items"`
	Pagination Pagination             `json:"pagination"`
}

type ProgramAnalyticsItem struct {
	ProgramID             uuid.UUID  `json:"program_id"`
	Name                  string     `json:"name"`
	NameRu                string     `json:"name_ru"`
	Status                string     `json:"status"`
	TrainingDaysCount     int        `json:"training_days_count"`
	ActiveClientsCount    int        `json:"active_clients_count"`
	TotalCompletions      int        `json:"total_completions"`
	CompletionsLast30Days int        `json:"completions_last_30_days"`
	AvgCompletionPercent  *float64   `json:"avg_completion_percent"`
	LastActivityAt        *time.Time `json:"last_activity_at"`
}

// ProgramAnalytics is the detail payload for one program.
type ProgramAnalytics struct {
	Program ProgramHeader      `json:"program"`
	Summary ProgramSummary     `json:"summary"`
	Clients []ProgramClient    `json:"clients"`
	Weeks   []ProgramWeekStats `json:"weeks"`
}

type ProgramHeader struct {
	ProgramID           uuid.UUID `json:"program_id"`
	Name                string    `json:"name"`
	NameRu              string    `json:"name_ru"`
	Status              string    `json:"status"`
	LatestVersionNumber int       `json:"latest_version_number"`
	TrainingDaysCount   int       `json:"training_days_count"`
}

type ProgramSummary struct {
	ActiveClientsCount    int        `json:"active_clients_count"`
	TotalCompletions      int        `json:"total_completions"`
	CompletionsLast30Days int        `json:"completions_last_30_days"`
	AvgCompletionPercent  *float64   `json:"avg_completion_percent"`
	LastActivityAt        *time.Time `json:"last_activity_at"`
}

type ProgramClient struct {
	ClientUserID      uuid.UUID  `json:"client_user_id"`
	DisplayName       string     `json:"display_name"`
	AvatarURL         string     `json:"avatar_url"`
	AssignedAt        time.Time  `json:"assigned_at"`
	IsBehindLatest    bool       `json:"is_behind_latest"`
	CompletedDays     int        `json:"completed_days"`
	TotalTrainingDays int        `json:"total_training_days"`
	CompletionPercent float64    `json:"completion_percent"`
	LastCompletedAt   *time.Time `json:"last_completed_at"`
}

type ProgramWeekStats struct {
	WeekNumber           int `json:"week_number"`
	CompletionsCount     int `json:"completions_count"`
	DistinctClientsCount int `json:"distinct_clients_count"`
}
