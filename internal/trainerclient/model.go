package trainerclient

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInviteNotFound      = errors.New("invite not found")
	ErrInviteExpired       = errors.New("invite expired")
	ErrInviteConsumed      = errors.New("invite already consumed")
	ErrInviteNotConfigured = errors.New("telegram invite not configured")
)

type Invite struct {
	ID        uuid.UUID `json:"id"`
	InviteURL string    `json:"invite_url"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type ClientProgramSummary struct {
	ProgramID        uuid.UUID `json:"program_id"`
	ProgramVersionID uuid.UUID `json:"program_version_id"`
	AssignedAt       time.Time `json:"assigned_at"`
}

type Client struct {
	ClientUserID      uuid.UUID             `json:"client_user_id"`
	DisplayName       string                `json:"display_name"`
	Status            string                `json:"status"`
	LinkedAt          time.Time             `json:"linked_at"`
	ProgramAssignment *ClientProgramSummary `json:"program_assignment"`
}

type ClientListResult struct {
	Items []Client `json:"items"`
}

type AcceptInviteRequest struct {
	Token          string `json:"token"`
	TelegramUserID string `json:"telegram_user_id"`
	DisplayName    string `json:"display_name"`
}

type AcceptInviteResult struct {
	UserID             uuid.UUID `json:"user_id"`
	TrainerID          uuid.UUID `json:"trainer_id"`
	TrainerDisplayName string    `json:"trainer_display_name"`
	AlreadyLinked      bool      `json:"already_linked"`
}
