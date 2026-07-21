//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/analytics"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/workoutcomment"
)

func TestWorkoutCommentStore_createConflictAndFeed(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	q := sqlc.New(pool)

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "wcc-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "wcc-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}
	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("trainer id: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerID, clientUserID); err != nil {
		t.Fatalf("link: %v", err)
	}

	insertCompletion := func(resultText string) uuid.UUID {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `
			INSERT INTO mentorix.client_workout_completions
				(client_user_id, trainer_id, completed_at, completion_cycle_id, day_key,
				 week_number, day_number, program_name, program_name_ru, day_snapshot, result_text, source)
			VALUES ($1, $2, now(), $3, $4, 2, 3, 'Strength', 'Сила', '{}'::jsonb, $5, 'telegram')
			RETURNING id
		`, clientUserID, trainerID, uuid.New(), uuid.New(), resultText).Scan(&id)
		if err != nil {
			t.Fatalf("insert completion: %v", err)
		}
		return id
	}
	commentedID := insertCompletion("присед 5х5 90 кг")
	uncommentedID := insertCompletion("жим 5х5 70 кг")

	svc := workoutcomment.NewService(pool)

	comment, err := svc.CreateComment(ctx, trainerUserID, clientUserID, commentedID, "Отличная работа!")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if comment.ID == uuid.Nil || comment.Text != "Отличная работа!" || comment.CreatedAt.IsZero() {
		t.Fatalf("comment = %+v", comment)
	}

	// One reply per completion.
	_, err = svc.CreateComment(ctx, trainerUserID, clientUserID, commentedID, "ещё раз")
	if !errors.Is(err, workoutcomment.ErrCommentExists) {
		t.Fatalf("second comment err = %v, want ErrCommentExists", err)
	}

	// Completion outside the trainer/client pair is invisible.
	_, err = svc.CreateComment(ctx, trainerUserID, uuid.New(), commentedID, "чужой клиент")
	if !errors.Is(err, workoutcomment.ErrCompletionNotFound) {
		t.Fatalf("foreign client err = %v, want ErrCompletionNotFound", err)
	}
	_, err = svc.CreateComment(ctx, trainerUserID, clientUserID, uuid.New(), "нет такой тренировки")
	if !errors.Is(err, workoutcomment.ErrCompletionNotFound) {
		t.Fatalf("missing completion err = %v, want ErrCompletionNotFound", err)
	}

	// The reply shows up in the trainer completions feed.
	analyticsSvc := analytics.NewService(pool, "test-jwt-secret-at-least-32-chars-long")
	feed, err := analyticsSvc.ClientCompletions(ctx, trainerUserID, clientUserID, analytics.CompletionsParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ClientCompletions: %v", err)
	}
	if len(feed.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(feed.Items))
	}
	byID := map[uuid.UUID]analytics.CompletionItem{}
	for _, item := range feed.Items {
		byID[item.ID] = item
	}
	withComment := byID[commentedID]
	if len(withComment.Comments) != 1 {
		t.Fatalf("comments = %+v, want 1", withComment.Comments)
	}
	if withComment.Comments[0].ID != comment.ID || withComment.Comments[0].Text != "Отличная работа!" {
		t.Fatalf("feed comment = %+v", withComment.Comments[0])
	}
	without := byID[uncommentedID]
	if without.Comments == nil || len(without.Comments) != 0 {
		t.Fatalf("uncommented comments = %#v, want empty non-nil slice", without.Comments)
	}
}
