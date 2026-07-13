//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/workoutcompletion"
)

func TestWorkoutCompletionStore_insertExistsAndConflict(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	q := sqlc.New(pool)

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "wc-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "wc-client@test.com", pwHash, "")
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

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	now := time.Now().UTC()
	version, err := q.InsertProgramVersion(ctx, sqlc.InsertProgramVersionParams{
		ProgramID:          pgconv.ToPGUUID(draft.ID),
		VersionNumber:      1,
		PublishedAt:        now,
		PublishedBy:        pgconv.ToPGUUID(trainerUserID),
		Name:               "WC Program",
		NameRu:             "WC",
		ContentFingerprint: "wc-fp",
	})
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	cycleID := uuid.New()
	assignment, err := q.InsertProgramAssignment(ctx, sqlc.InsertProgramAssignmentParams{
		ProgramID:         pgconv.ToPGUUID(draft.ID),
		ProgramVersionID:  version.ID,
		TrainerID:         trainerID,
		ClientUserID:      pgconv.ToPGUUID(clientUserID),
		Status:            "active",
		AssignedAt:        now,
		CreatedBy:         pgconv.ToPGUUID(trainerUserID),
		ModifiedAt:        now,
		ModifiedBy:        pgconv.ToPGUUID(trainerUserID),
		CompletionCycleID: pgconv.ToPGUUID(cycleID),
	})
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}

	dayKey := uuid.New()
	programID := draft.ID
	versionID := pgconv.FromPGUUID(version.ID)
	assignmentID := pgconv.FromPGUUID(assignment.ID)

	svc := workoutcompletion.NewService(pool)
	ok, err := svc.IsCompleted(ctx, cycleID, dayKey)
	if err != nil || ok {
		t.Fatalf("before insert: ok=%v err=%v", ok, err)
	}

	got, err := svc.Complete(ctx, workoutcompletion.CompleteInput{
		ClientUserID:        clientUserID,
		TrainerID:           pgconv.FromPGUUID(trainerID),
		ProgramID:           programID,
		ProgramVersionID:    versionID,
		ProgramAssignmentID: assignmentID,
		CompletionCycleID:   cycleID,
		DayKey:              dayKey,
		WeekNumber:          1,
		DayNumber:           2,
		ProgramName:         "WC Program",
		ProgramNameRu:       "WC",
		Day: program.Day{
			Blocks: []program.DayBlock{{
				BlockType: program.BlockTypeSingle,
				Exercises: []program.DayExercise{{ExerciseName: "Squat"}},
			}},
		},
		ResultText: "felt strong",
		Source:     workoutcompletion.SourceTelegram,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.ResultText != "felt strong" || got.ID == uuid.Nil {
		t.Fatalf("got %+v", got)
	}

	ok, err = svc.IsCompleted(ctx, cycleID, dayKey)
	if err != nil || !ok {
		t.Fatalf("after insert: ok=%v err=%v", ok, err)
	}
	keys, err := svc.CompletedDayKeys(ctx, cycleID)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := keys[dayKey]; !has {
		t.Fatalf("keys=%v", keys)
	}

	_, err = svc.Complete(ctx, workoutcompletion.CompleteInput{
		ClientUserID:      clientUserID,
		TrainerID:         pgconv.FromPGUUID(trainerID),
		CompletionCycleID: cycleID,
		DayKey:            dayKey,
		WeekNumber:        1,
		DayNumber:         2,
		ResultText:        "again",
	})
	if !errors.Is(err, workoutcompletion.ErrAlreadyCompleted) {
		t.Fatalf("conflict err=%v", err)
	}
}
