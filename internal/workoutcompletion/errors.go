package workoutcompletion

import "errors"

var (
	ErrValidation       = errors.New("validation failed")
	ErrAlreadyCompleted = errors.New("workout already completed")
	ErrNotFound         = errors.New("not found")
)

const (
	SourceTelegram   = "telegram"
	MaxResultTextLen = 1000
)
