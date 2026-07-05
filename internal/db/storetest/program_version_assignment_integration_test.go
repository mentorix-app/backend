//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
)

func TestProgramVersionAndAssignment_oneActivePerTrainerClient(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	q := sqlc.New(pool)

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "version-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "version-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client user: %v", err)
	}

	trainerID, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("GetTrainerIDByUserID(trainer): %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerID, clientUserID)
	if err != nil {
		t.Fatalf("insert trainer_clients: %v", err)
	}

	progStore := program.NewStore(pool)
	draft, err := progStore.CreateDraft(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	now := time.Now().UTC()
	version, err := q.InsertProgramVersion(ctx, sqlc.InsertProgramVersionParams{
		ProgramID:          pgconv.ToPGUUID(draft.ID),
		VersionNumber:      1,
		PublishedAt:        now,
		PublishedBy:        pgconv.ToPGUUID(trainerUserID),
		Name:               "Test Program",
		NameRu:             "",
		Description:        "",
		DescriptionRu:      "",
		Category:           ptrString("muscle_gain"),
		Difficulty:         ptrString("beginner"),
		PreviewImageUrl:    "",
		ContentFingerprint: "test-fingerprint",
	})
	if err != nil {
		t.Fatalf("InsertProgramVersion: %v", err)
	}

	programB, err := progStore.CreateDraft(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateDraft program B: %v", err)
	}
	versionB, err := q.InsertProgramVersion(ctx, sqlc.InsertProgramVersionParams{
		ProgramID:          pgconv.ToPGUUID(programB.ID),
		VersionNumber:      1,
		PublishedAt:        now,
		PublishedBy:        pgconv.ToPGUUID(trainerUserID),
		Name:               "Program B",
		ContentFingerprint: "test-fingerprint-b",
	})
	if err != nil {
		t.Fatalf("InsertProgramVersion B: %v", err)
	}

	trainerUserPG := pgconv.ToPGUUID(trainerUserID)

	_, err = q.InsertProgramAssignment(ctx, sqlc.InsertProgramAssignmentParams{
		ProgramID:        pgconv.ToPGUUID(draft.ID),
		ProgramVersionID: version.ID,
		TrainerID:        trainerID,
		ClientUserID:     pgconv.ToPGUUID(clientUserID),
		Status:           "active",
		AssignedAt:       now,
		CreatedBy:        trainerUserPG,
		ModifiedAt:       now,
		ModifiedBy:       trainerUserPG,
	})
	if err != nil {
		t.Fatalf("InsertProgramAssignment: %v", err)
	}

	_, err = q.InsertProgramAssignment(ctx, sqlc.InsertProgramAssignmentParams{
		ProgramID:        pgconv.ToPGUUID(programB.ID),
		ProgramVersionID: versionB.ID,
		TrainerID:        trainerID,
		ClientUserID:     pgconv.ToPGUUID(clientUserID),
		Status:           "active",
		AssignedAt:       now,
		CreatedBy:        trainerUserPG,
		ModifiedAt:       now,
		ModifiedBy:       trainerUserPG,
	})
	if err == nil {
		t.Fatal("expected unique violation for second active assignment")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("InsertProgramAssignment second active: want unique violation 23505, got %v", err)
	}

	rows, err := q.CancelActiveProgramAssignmentsForTrainerClient(ctx, sqlc.CancelActiveProgramAssignmentsForTrainerClientParams{
		TrainerID:    trainerID,
		ClientUserID: pgconv.ToPGUUID(clientUserID),
		ModifiedAt:   now,
		ModifiedBy:   trainerUserPG,
	})
	if err != nil {
		t.Fatalf("CancelActiveProgramAssignmentsForTrainerClient: %v", err)
	}
	if rows != 1 {
		t.Fatalf("cancelled rows = %d, want 1", rows)
	}

	active, err := q.GetActiveProgramAssignmentByTrainerClient(ctx, sqlc.GetActiveProgramAssignmentByTrainerClientParams{
		TrainerID:    trainerID,
		ClientUserID: pgconv.ToPGUUID(clientUserID),
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("GetActiveProgramAssignmentByTrainerClient after cancel: %v", err)
	}
	_ = active

	reassigned, err := q.InsertProgramAssignment(ctx, sqlc.InsertProgramAssignmentParams{
		ProgramID:        pgconv.ToPGUUID(programB.ID),
		ProgramVersionID: versionB.ID,
		TrainerID:        trainerID,
		ClientUserID:     pgconv.ToPGUUID(clientUserID),
		Status:           "active",
		AssignedAt:       now,
		CreatedBy:        trainerUserPG,
		ModifiedAt:       now,
		ModifiedBy:       trainerUserPG,
	})
	if err != nil {
		t.Fatalf("InsertProgramAssignment after cancel: %v", err)
	}
	if pgconv.FromPGUUID(reassigned.ProgramID) != programB.ID {
		t.Errorf("program_id = %v, want %v", pgconv.FromPGUUID(reassigned.ProgramID), programB.ID)
	}

	versions, err := q.ListProgramVersionsByProgramID(ctx, pgconv.ToPGUUID(draft.ID))
	if err != nil {
		t.Fatalf("ListProgramVersionsByProgramID: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("versions len = %d, want 1", len(versions))
	}
	if versions[0].AssignmentCount != 0 {
		t.Errorf("v1 assignment_count = %d, want 0 after reassign", versions[0].AssignmentCount)
	}
}

func ptrString(s string) *string {
	return &s
}
