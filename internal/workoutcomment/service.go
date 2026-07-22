package workoutcomment

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/telegramnotify"
)

type commentStore interface {
	TrainerIDForUser(ctx context.Context, trainerUserID uuid.UUID) (uuid.UUID, error)
	CompletionForComment(ctx context.Context, completionID, trainerID, clientUserID uuid.UUID) (CompletionInfo, error)
	CreateComment(ctx context.Context, completionID, trainerID uuid.UUID, text string) (Comment, error)
}

// CommentNotifier pushes the trainer reply to the client (best-effort).
type CommentNotifier interface {
	NotifyWorkoutCommented(ctx context.Context, clientUserID, trainerID uuid.UUID, comment telegramnotify.WorkoutComment) error
}

type Service struct {
	store    commentStore
	notifier CommentNotifier
}

type ServiceOption func(*Service)

func WithCommentNotifier(n CommentNotifier) ServiceOption {
	return func(s *Service) { s.notifier = n }
}

func NewService(pool *pgxpool.Pool, opts ...ServiceOption) *Service {
	return NewServiceWithStore(NewStore(pool), opts...)
}

func NewServiceWithStore(store commentStore, opts ...ServiceOption) *Service {
	s := &Service{store: store}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) CreateComment(ctx context.Context, trainerUserID, clientUserID, completionID uuid.UUID, text string) (Comment, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Comment{}, fmt.Errorf("%w: text required", ErrValidation)
	}
	if utf8.RuneCountInString(text) > MaxCommentTextLen {
		return Comment{}, fmt.Errorf("%w: text too long", ErrValidation)
	}

	trainerID, err := s.store.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		return Comment{}, err
	}
	info, err := s.store.CompletionForComment(ctx, completionID, trainerID, clientUserID)
	if err != nil {
		return Comment{}, err
	}
	comment, err := s.store.CreateComment(ctx, completionID, trainerID, text)
	if err != nil {
		return Comment{}, err
	}

	if s.notifier != nil {
		// Best-effort push; failures are logged inside the notifier.
		_ = s.notifier.NotifyWorkoutCommented(ctx, info.ClientUserID, trainerID, telegramnotify.WorkoutComment{
			ProgramName:   info.ProgramName,
			ProgramNameRu: info.ProgramNameRu,
			WeekNumber:    info.WeekNumber,
			DayNumber:     info.DayNumber,
			ResultText:    info.ResultText,
			CommentText:   comment.Text,
		})
	}
	return comment, nil
}
