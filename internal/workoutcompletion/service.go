package workoutcompletion

import (
	"context"
	"errors"
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

// assignmentInserter is the conditional insert that CompleteForAssignment needs. It is
// separate from completionStore so a store written for Complete alone stays valid.
type assignmentInserter interface {
	InsertForAssignment(ctx context.Context, c Completion) (Completion, error)
}

type Service struct {
	store       completionStore
	assignments assignmentInserter
}

func NewService(pool *pgxpool.Pool) *Service {
	return New(NewStore(pool))
}

// New wraps store. CompleteForAssignment works when store also has InsertForAssignment.
func New(store completionStore) *Service {
	s := &Service{store: store}
	s.assignments, _ = store.(assignmentInserter)
	return s
}

func (s *Service) IsCompleted(ctx context.Context, completionCycleID, dayKey uuid.UUID) (bool, error) {
	return s.store.Exists(ctx, completionCycleID, dayKey)
}

func (s *Service) CompletedDayKeys(ctx context.Context, completionCycleID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	return s.store.ListCompletedDayKeys(ctx, completionCycleID)
}

func (s *Service) Complete(ctx context.Context, in CompleteInput) (Completion, error) {
	c, err := buildCompletion(in)
	if err != nil {
		return Completion{}, err
	}
	return s.store.Insert(ctx, c)
}

// CompleteForAssignment is Complete for a caller that read the assignment earlier: the
// row is written only while the assignment still has in's cycle, version and client,
// otherwise ErrAssignmentChanged. The program of the row is the assignment's.
func (s *Service) CompleteForAssignment(ctx context.Context, in CompleteInput) (Completion, error) {
	c, err := buildCompletion(in)
	if err != nil {
		return Completion{}, err
	}
	if s.assignments == nil {
		return Completion{}, errors.New("completion store has no conditional insert")
	}
	return s.assignments.InsertForAssignment(ctx, c)
}

func buildCompletion(in CompleteInput) (Completion, error) {
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
	if source != SourceTelegram && source != SourceApp {
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
	return c, nil
}
