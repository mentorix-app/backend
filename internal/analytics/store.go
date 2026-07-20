package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

type Store struct {
	q *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: sqlc.New(pool)}
}

func (s *Store) TrainerIDForUser(ctx context.Context, trainerUserID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		return uuid.Nil, err
	}
	return pgconv.FromPGUUID(id), nil
}

func (s *Store) ClientHeader(ctx context.Context, trainerID, clientUserID uuid.UUID) (sqlc.GetClientAnalyticsHeaderRow, error) {
	return s.q.GetClientAnalyticsHeader(ctx, sqlc.GetClientAnalyticsHeaderParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
}

func (s *Store) CurrentAssignment(ctx context.Context, trainerID, clientUserID uuid.UUID) (sqlc.GetClientCurrentAssignmentAnalyticsRow, error) {
	return s.q.GetClientCurrentAssignmentAnalytics(ctx, sqlc.GetClientCurrentAssignmentAnalyticsParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
}

func (s *Store) CycleWeekProgress(ctx context.Context, programVersionID, completionCycleID uuid.UUID) ([]sqlc.ListCycleWeekProgressRow, error) {
	return s.q.ListCycleWeekProgress(ctx, sqlc.ListCycleWeekProgressParams{
		ProgramVersionID:  pgconv.ToPGUUID(programVersionID),
		CompletionCycleID: pgconv.ToPGUUID(completionCycleID),
	})
}

func (s *Store) ActivityStats(ctx context.Context, trainerID, clientUserID uuid.UUID, since7d, since30d time.Time) (sqlc.GetClientActivityStatsRow, error) {
	return s.q.GetClientActivityStats(ctx, sqlc.GetClientActivityStatsParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
		Since7d:      since7d,
		Since30d:     since30d,
	})
}

func (s *Store) CompletionWeeks(ctx context.Context, trainerID, clientUserID uuid.UUID) ([]time.Time, error) {
	rows, err := s.q.ListClientCompletionWeeks(ctx, sqlc.ListClientCompletionWeeksParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
	if err != nil {
		return nil, err
	}
	// week_start is truncated in UTC on the DB side; reattach the UTC location.
	out := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		if !row.Valid {
			continue
		}
		t := row.Time
		out = append(out, time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))
	}
	return out, nil
}

func (s *Store) ProgramActivity(ctx context.Context, trainerID, clientUserID uuid.UUID) ([]sqlc.ListClientProgramActivityRow, error) {
	return s.q.ListClientProgramActivity(ctx, sqlc.ListClientProgramActivityParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
}

func (s *Store) CountClientCompletions(ctx context.Context, trainerID, clientUserID uuid.UUID, params CompletionsParams) (int, error) {
	total, err := s.q.CountClientCompletions(ctx, sqlc.CountClientCompletionsParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
		FromAt:       toPGTimestamptz(params.From),
		ToAt:         toPGTimestamptz(params.To),
	})
	return int(total), err
}

func (s *Store) ListClientCompletions(ctx context.Context, trainerID, clientUserID uuid.UUID, params CompletionsParams) ([]sqlc.ListClientCompletionsRow, error) {
	return s.q.ListClientCompletions(ctx, sqlc.ListClientCompletionsParams{
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
		FromAt:       toPGTimestamptz(params.From),
		ToAt:         toPGTimestamptz(params.To),
		Limit:        int32(params.Limit),
		Offset:       int32(params.Offset()),
	})
}

func (s *Store) CountTrainerPrograms(ctx context.Context, createdBy uuid.UUID) (int, error) {
	total, err := s.q.CountPrograms(ctx, sqlc.CountProgramsParams{
		FilterCreatedBy: pgconv.ToPGUUID(createdBy),
	})
	return int(total), err
}

func (s *Store) ListProgramsAnalytics(ctx context.Context, createdBy, trainerID uuid.UUID, since30d time.Time, params ProgramsParams) ([]sqlc.ListTrainerProgramsAnalyticsRow, error) {
	return s.q.ListTrainerProgramsAnalytics(ctx, sqlc.ListTrainerProgramsAnalyticsParams{
		CreatedBy: pgconv.ToPGUUID(createdBy),
		TrainerID: pgconv.ToPGUUID(trainerID),
		Since30d:  since30d,
		SortBy:    params.SortBy,
		SortOrder: params.SortOrder,
		Limit:     int32(params.Limit),
		Offset:    int32(params.Offset()),
	})
}

func (s *Store) ProgramHeader(ctx context.Context, programID uuid.UUID) (sqlc.GetProgramAnalyticsHeaderRow, error) {
	return s.q.GetProgramAnalyticsHeader(ctx, pgconv.ToPGUUID(programID))
}

func (s *Store) ProgramCompletionTotals(ctx context.Context, trainerID, programID uuid.UUID, since30d time.Time) (sqlc.GetProgramCompletionTotalsRow, error) {
	return s.q.GetProgramCompletionTotals(ctx, sqlc.GetProgramCompletionTotalsParams{
		TrainerID: pgconv.ToPGUUID(trainerID),
		ProgramID: pgconv.ToPGUUID(programID),
		Since30d:  since30d,
	})
}

func (s *Store) ProgramClients(ctx context.Context, programID uuid.UUID) ([]sqlc.ListProgramAnalyticsClientsRow, error) {
	return s.q.ListProgramAnalyticsClients(ctx, pgconv.ToPGUUID(programID))
}

func (s *Store) ProgramWeeks(ctx context.Context, programID uuid.UUID) ([]sqlc.ListProgramAnalyticsWeeksRow, error) {
	return s.q.ListProgramAnalyticsWeeks(ctx, pgconv.ToPGUUID(programID))
}

func toPGTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func timePtrFromAny(v any) *time.Time {
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return nil
		}
		u := t.UTC()
		return &u
	case *time.Time:
		if t == nil {
			return nil
		}
		u := t.UTC()
		return &u
	case pgtype.Timestamptz:
		if !t.Valid {
			return nil
		}
		u := t.Time.UTC()
		return &u
	default:
		return nil
	}
}

func uuidPtrFromPG(id pgtype.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	u := pgconv.FromPGUUID(id)
	return &u
}
