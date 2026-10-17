package workoutcompletion

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

func (s *Store) Exists(ctx context.Context, completionCycleID, dayKey uuid.UUID) (bool, error) {
	ok, err := s.q.ClientWorkoutCompletionExists(ctx, sqlc.ClientWorkoutCompletionExistsParams{
		CompletionCycleID: pgconv.ToPGUUID(completionCycleID),
		DayKey:            pgconv.ToPGUUID(dayKey),
	})
	if err != nil {
		return false, fmt.Errorf("completion exists: %w", err)
	}
	return ok, nil
}

func (s *Store) ListCompletedDayKeys(ctx context.Context, completionCycleID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	rows, err := s.q.ListCompletedDayKeysByCycle(ctx, pgconv.ToPGUUID(completionCycleID))
	if err != nil {
		return nil, fmt.Errorf("list completed day keys: %w", err)
	}
	out := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		out[pgconv.FromPGUUID(row)] = struct{}{}
	}
	return out, nil
}

func (s *Store) Insert(ctx context.Context, c Completion) (Completion, error) {
	row, err := s.q.InsertClientWorkoutCompletion(ctx, sqlc.InsertClientWorkoutCompletionParams{
		ClientUserID:        pgconv.ToPGUUID(c.ClientUserID),
		TrainerID:           pgconv.ToPGUUID(c.TrainerID),
		CompletedAt:         c.CompletedAt,
		ProgramID:           optionalPGUUID(c.ProgramID),
		ProgramVersionID:    optionalPGUUID(c.ProgramVersionID),
		ProgramAssignmentID: optionalPGUUID(c.ProgramAssignmentID),
		CompletionCycleID:   pgconv.ToPGUUID(c.CompletionCycleID),
		DayKey:              pgconv.ToPGUUID(c.DayKey),
		WeekNumber:          int32(c.WeekNumber),
		DayNumber:           int32(c.DayNumber),
		ProgramName:         c.ProgramName,
		ProgramNameRu:       c.ProgramNameRu,
		DaySnapshot:         c.DaySnapshot,
		ResultText:          c.ResultText,
		Source:              c.Source,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Completion{}, ErrAlreadyCompleted
		}
		return Completion{}, fmt.Errorf("insert completion: %w", err)
	}
	return completionFromRow(row), nil
}

// InsertForAssignment writes c only while c's assignment still has the cycle, version,
// client and trainer c names and the trainer's link is active (one statement, no lock on
// the assignment row). Nothing written is ErrAssignmentChanged. So is a foreign key
// that no longer holds (23503): a reassignment can commit after the statement read the
// assignment and delete the old version before its foreign key is checked. The
// program of the row comes from the assignment, not from c.
func (s *Store) InsertForAssignment(ctx context.Context, c Completion) (Completion, error) {
	if c.ProgramAssignmentID == nil || c.ProgramVersionID == nil {
		return Completion{}, fmt.Errorf("%w: assignment and version required", ErrValidation)
	}
	row, err := s.q.InsertClientWorkoutCompletionForAssignment(ctx, sqlc.InsertClientWorkoutCompletionForAssignmentParams{
		CompletedAt:         c.CompletedAt,
		DayKey:              pgconv.ToPGUUID(c.DayKey),
		WeekNumber:          int32(c.WeekNumber),
		DayNumber:           int32(c.DayNumber),
		ProgramName:         c.ProgramName,
		ProgramNameRu:       c.ProgramNameRu,
		DaySnapshot:         c.DaySnapshot,
		ResultText:          c.ResultText,
		Source:              c.Source,
		ProgramAssignmentID: pgconv.ToPGUUID(*c.ProgramAssignmentID),
		ClientUserID:        pgconv.ToPGUUID(c.ClientUserID),
		TrainerID:           pgconv.ToPGUUID(c.TrainerID),
		ProgramVersionID:    pgconv.ToPGUUID(*c.ProgramVersionID),
		CompletionCycleID:   pgconv.ToPGUUID(c.CompletionCycleID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Completion{}, ErrAssignmentChanged
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return Completion{}, ErrAlreadyCompleted
			case "23503":
				return Completion{}, ErrAssignmentChanged
			}
		}
		return Completion{}, fmt.Errorf("insert completion for assignment: %w", err)
	}
	return completionFromRow(row), nil
}

func optionalPGUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgconv.ToPGUUID(*id)
}

func completionFromRow(row sqlc.MentorixClientWorkoutCompletion) Completion {
	c := Completion{
		ID:                pgconv.FromPGUUID(row.ID),
		ClientUserID:      pgconv.FromPGUUID(row.ClientUserID),
		TrainerID:         pgconv.FromPGUUID(row.TrainerID),
		CompletedAt:       row.CompletedAt.UTC(),
		CompletionCycleID: pgconv.FromPGUUID(row.CompletionCycleID),
		DayKey:            pgconv.FromPGUUID(row.DayKey),
		WeekNumber:        int(row.WeekNumber),
		DayNumber:         int(row.DayNumber),
		ProgramName:       row.ProgramName,
		ProgramNameRu:     row.ProgramNameRu,
		DaySnapshot:       row.DaySnapshot,
		ResultText:        row.ResultText,
		Source:            row.Source,
		CreatedAt:         row.CreatedAt.UTC(),
	}
	if row.ProgramID.Valid {
		id := pgconv.FromPGUUID(row.ProgramID)
		c.ProgramID = &id
	}
	if row.ProgramVersionID.Valid {
		id := pgconv.FromPGUUID(row.ProgramVersionID)
		c.ProgramVersionID = &id
	}
	if row.ProgramAssignmentID.Valid {
		id := pgconv.FromPGUUID(row.ProgramAssignmentID)
		c.ProgramAssignmentID = &id
	}
	return c
}
