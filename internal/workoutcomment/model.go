package workoutcomment

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrValidation         = errors.New("validation failed")
	ErrForbidden          = errors.New("forbidden")
	ErrCompletionNotFound = errors.New("completion not found")
	ErrCommentExists      = errors.New("comment already exists")
)

const MaxCommentTextLen = 2000

// Comment is the trainer reply returned by the API.
type Comment struct {
	ID        uuid.UUID `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// CompletionInfo carries the completion snapshot fields needed for the
// Telegram notification about a new trainer comment.
type CompletionInfo struct {
	ID            uuid.UUID
	ClientUserID  uuid.UUID
	WeekNumber    int
	DayNumber     int
	ProgramName   string
	ProgramNameRu string
	ResultText    string
}
