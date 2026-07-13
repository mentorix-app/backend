package workoutcompletion

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type completionStore interface {
	Exists(ctx context.Context, completionCycleID, dayKey uuid.UUID) (bool, error)
	ListCompletedDayKeys(ctx context.Context, completionCycleID uuid.UUID) (map[uuid.UUID]struct{}, error)
	Insert(ctx context.Context, c Completion) (Completion, error)
}

type Service struct {
	store completionStore
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{store: NewStore(pool)}
}

func New(store completionStore) *Service {
	return &Service{store: store}
}

func (s *Service) IsCompleted(ctx context.Context, completionCycleID, dayKey uuid.UUID) (bool, error) {
	return s.store.Exists(ctx, completionCycleID, dayKey)
}

func (s *Service) CompletedDayKeys(ctx context.Context, completionCycleID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	return s.store.ListCompletedDayKeys(ctx, completionCycleID)
}

func (s *Service) Complete(ctx context.Context, in CompleteInput) (Completion, error) {
	text := strings.TrimSpace(in.ResultText)
	if text == "" {
		return Completion{}, fmt.Errorf("%w: result_text required", ErrValidation)
	}
	if utf8.RuneCountInString(text) > MaxResultTextLen {
		return Completion{}, fmt.Errorf("%w: result_text too long", ErrValidation)
	}
	source := in.Source
	if source == "" {
		source = SourceTelegram
	}
	if source != SourceTelegram {
		return Completion{}, fmt.Errorf("%w: invalid source", ErrValidation)
	}
	if in.CompletionCycleID == uuid.Nil || in.DayKey == uuid.Nil {
		return Completion{}, fmt.Errorf("%w: cycle and day_key required", ErrValidation)
	}

	snapshot, err := buildDaySnapshot(in.Day)
	if err != nil {
		return Completion{}, fmt.Errorf("day snapshot: %w", err)
	}

	now := time.Now().UTC()
	programID := in.ProgramID
	versionID := in.ProgramVersionID
	assignmentID := in.ProgramAssignmentID
	c := Completion{
		ClientUserID:        in.ClientUserID,
		TrainerID:           in.TrainerID,
		CompletedAt:         now,
		ProgramID:           &programID,
		ProgramVersionID:    &versionID,
		ProgramAssignmentID: &assignmentID,
		CompletionCycleID:   in.CompletionCycleID,
		DayKey:              in.DayKey,
		WeekNumber:          in.WeekNumber,
		DayNumber:           in.DayNumber,
		ProgramName:         in.ProgramName,
		ProgramNameRu:       in.ProgramNameRu,
		DaySnapshot:         []byte(snapshot),
		ResultText:          text,
		Source:              source,
	}
	return s.store.Insert(ctx, c)
}
