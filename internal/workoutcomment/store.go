package workoutcomment

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrForbidden
		}
		return uuid.Nil, fmt.Errorf("trainer id: %w", err)
	}
	return pgconv.FromPGUUID(id), nil
}

func (s *Store) CompletionForComment(ctx context.Context, completionID, trainerID, clientUserID uuid.UUID) (CompletionInfo, error) {
	row, err := s.q.GetCompletionForComment(ctx, sqlc.GetCompletionForCommentParams{
		ID:           pgconv.ToPGUUID(completionID),
		TrainerID:    pgconv.ToPGUUID(trainerID),
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CompletionInfo{}, ErrCompletionNotFound
		}
		return CompletionInfo{}, fmt.Errorf("completion for comment: %w", err)
	}
	return CompletionInfo{
		ID:            pgconv.FromPGUUID(row.ID),
		ClientUserID:  pgconv.FromPGUUID(row.ClientUserID),
		WeekNumber:    int(row.WeekNumber),
		DayNumber:     int(row.DayNumber),
		ProgramName:   row.ProgramName,
		ProgramNameRu: row.ProgramNameRu,
		ResultText:    row.ResultText,
	}, nil
}

func (s *Store) CreateComment(ctx context.Context, completionID, trainerID uuid.UUID, text string) (Comment, error) {
	row, err := s.q.CreateCompletionComment(ctx, sqlc.CreateCompletionCommentParams{
		ClientWorkoutCompletionID: pgconv.ToPGUUID(completionID),
		TrainerID:                 pgconv.ToPGUUID(trainerID),
		CommentText:               text,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Comment{}, ErrCommentExists
		}
		return Comment{}, fmt.Errorf("create comment: %w", err)
	}
	return Comment{
		ID:        pgconv.FromPGUUID(row.ID),
		Text:      row.CommentText,
		CreatedAt: row.CreatedAt.UTC(),
	}, nil
}
