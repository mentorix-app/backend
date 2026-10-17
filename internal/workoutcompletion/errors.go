package workoutcompletion

import "errors"

var (
	ErrValidation       = errors.New("validation failed")
	ErrAlreadyCompleted = errors.New("workout already completed")
	ErrNotFound         = errors.New("not found")
	// ErrAssignmentChanged means the assignment no longer has the cycle, version and
	// client the write named, so nothing was written.
	ErrAssignmentChanged = errors.New("assignment changed")
)

const (
	SourceTelegram   = "telegram"
	SourceApp        = "app"
	MaxResultTextLen = 1000
)
