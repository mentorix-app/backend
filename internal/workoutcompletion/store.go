package workoutcompletion

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
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
