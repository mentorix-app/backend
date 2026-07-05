package program

import (
	"time"

	"github.com/google/uuid"
)

type AssignmentStatus string

const (
	AssignmentStatusActive    AssignmentStatus = "active"
	AssignmentStatusCompleted AssignmentStatus = "completed"
	AssignmentStatusCancelled AssignmentStatus = "cancelled"
)

type Assignment struct {
	ID               uuid.UUID        `json:"id"`
	ProgramID        uuid.UUID        `json:"program_id"`
	ProgramVersionID uuid.UUID        `json:"program_version_id"`
	TrainerID        uuid.UUID        `json:"trainer_id"`
	ClientUserID     uuid.UUID        `json:"client_user_id"`
	Status           AssignmentStatus `json:"status"`
	AssignedAt       time.Time        `json:"assigned_at"`
	CreatedAt        time.Time        `json:"created_at"`
	ClientPlanAt     *time.Time       `json:"client_plan_at,omitempty"`
	IsBehindLatest   *bool            `json:"is_behind_latest,omitempty"`
}

type AssignmentListResult struct {
	Items                  []Assignment `json:"items"`
	LatestProgramVersionID *uuid.UUID   `json:"latest_program_version_id"`
}

type AssignmentSyncRequest struct {
	AllActive     *bool       `json:"all_active"`
	AssignmentIDs []uuid.UUID `json:"assignment_ids"`
}

type AssignmentSyncSkipped struct {
	AssignmentID uuid.UUID `json:"assignment_id"`
	Reason       string    `json:"reason"`
}

type AssignmentSyncResult struct {
	Synced  []Assignment            `json:"synced"`
	Skipped []AssignmentSyncSkipped `json:"skipped"`
}

type VersionSummary struct {
	ID              uuid.UUID `json:"id"`
	VersionNumber   int       `json:"version_number"`
	PublishedAt     time.Time `json:"published_at"`
	CreatedAt       time.Time `json:"created_at"`
	AssignmentCount int       `json:"assignment_count"`
	CanDelete       bool      `json:"can_delete"`
}

type VersionListResult struct {
	Items []VersionSummary `json:"items"`
}

type VersionCleanupSkipped struct {
	VersionID uuid.UUID `json:"version_id"`
	Reason    string    `json:"reason"`
}

type VersionCleanupResult struct {
	DeletedVersionIDs []uuid.UUID             `json:"deleted_version_ids"`
	Skipped           []VersionCleanupSkipped `json:"skipped"`
}

type SetClientProgramAssignmentRequest struct {
	ProgramID *uuid.UUID `json:"program_id"`
}
