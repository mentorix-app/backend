//go:build integration

package storetest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

func TestTelegramClient_programAndToday(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "tg-trainer@test.com", pwHash, "Coach Max")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	activeStore := trainerclient.NewMemoryActiveTrainerStore()
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, activeStore, nil)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
	accept, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: "777001",
		DisplayName:    "Client TG",
	})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	exStore := exercise.NewStore(pool)
	catalogExercise, err := exStore.Create(ctx, trainerUserID, exercise.UpsertInput{
		Name:        "Squat",
		NameRu:      "Присед",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	draft, err := progSvc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create program: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := 3, 10
	if _, err := createSingleBlock(ctx, progSvc, trainerUserID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: catalogExercise.ID,
		Sets:       &sets,
		Reps:       &reps,
	}); err != nil {
		t.Fatalf("create block: %v", err)
	}
	name := "Strength Plan"
	category := program.CategoryMuscleGain
	difficulty := exercise.DifficultyBeginner
	if _, err := progSvc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{
		Name:       &name,
		Category:   &category,
		Difficulty: &difficulty,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := progSvc.Publish(ctx, trainerUserID, draft.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	programID := draft.ID
	if _, err := svc.BulkSetClientProgramAssignment(ctx, trainerUserID, program.BulkSetClientProgramAssignmentRequest{
		ProgramID:     &programID,
		ClientUserIDs: []uuid.UUID{accept.UserID},
	}); err != nil {
		t.Fatalf("BulkSetClientProgramAssignment: %v", err)
	}

	trainers, err := svc.ListTelegramTrainers(ctx, "777001")
	if err != nil {
		t.Fatalf("ListTelegramTrainers: %v", err)
	}
	if len(trainers.Items) != 1 || !trainers.Items[0].HasProgram {
		t.Fatalf("trainers = %+v", trainers)
	}
	if trainers.ActiveTrainerID == nil || *trainers.ActiveTrainerID != accept.TrainerID {
		t.Fatalf("active_trainer_id = %v", trainers.ActiveTrainerID)
	}

	programView, err := svc.GetTelegramProgram(ctx, "777001", nil)
	if err != nil {
		t.Fatalf("GetTelegramProgram: %v", err)
	}
	if !programView.HasProgram || programView.Program == nil || len(programView.Program.Weeks) == 0 {
		t.Fatalf("program view = %+v", programView)
	}

	today, err := svc.GetTelegramToday(ctx, "777001", nil)
	if err != nil {
		t.Fatalf("GetTelegramToday: %v", err)
	}
	if !today.HasProgram || today.IsRestDay || len(today.Blocks) == 0 {
		t.Fatalf("today = %+v", today)
	}

	active, err := svc.GetTelegramActiveTrainer(ctx, "777001")
	if err != nil {
		t.Fatalf("GetTelegramActiveTrainer: %v", err)
	}
	if active.TrainerID != accept.TrainerID {
		t.Fatalf("active trainer = %v", active.TrainerID)
	}

	trainerID := accept.TrainerID
	programView, err = svc.GetTelegramProgram(ctx, "777001", &trainerID)
	if err != nil || !programView.HasProgram {
		t.Fatalf("GetTelegramProgram explicit trainer: %+v err=%v", programView, err)
	}
}

func TestTelegramClient_setActiveTrainerTwoTrainers(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerA, err := authStore.RegisterTrainerEmailPassword(ctx, "tg-active-a@test.com", pwHash, "Coach A")
	if err != nil {
		t.Fatalf("register trainer A: %v", err)
	}
	trainerB, err := authStore.RegisterTrainerEmailPassword(ctx, "tg-active-b@test.com", pwHash, "Coach B")
	if err != nil {
		t.Fatalf("register trainer B: %v", err)
	}

	progSvc := program.NewService(pool)
	activeStore := trainerclient.NewMemoryActiveTrainerStore()
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, activeStore, nil)

	inviteA, err := svc.CreateInvite(ctx, trainerA)
	if err != nil {
		t.Fatalf("CreateInvite A: %v", err)
	}
	inviteB, err := svc.CreateInvite(ctx, trainerB)
	if err != nil {
		t.Fatalf("CreateInvite B: %v", err)
	}
	tokenA := inviteTokenFromURL(inviteA.InviteURL)
	tokenB := inviteTokenFromURL(inviteB.InviteURL)

	for _, token := range []string{tokenA, tokenB} {
		if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
			Token:          token,
			TelegramUserID: "555001",
			DisplayName:    "Multi Coach Client",
		}); err != nil {
			t.Fatalf("AcceptInvite: %v", err)
		}
	}

	trainers, err := svc.ListTelegramTrainers(ctx, "555001")
	if err != nil {
		t.Fatalf("ListTelegramTrainers: %v", err)
	}
	if len(trainers.Items) != 2 {
		t.Fatalf("trainers = %+v", trainers)
	}

	var trainerBID uuid.UUID
	for _, item := range trainers.Items {
		if item.DisplayName == "Coach B" {
			trainerBID = item.TrainerID
			break
		}
	}
	if trainerBID == uuid.Nil {
		t.Fatalf("trainer B not in list: %+v", trainers.Items)
	}

	active, err := svc.SetTelegramActiveTrainer(ctx, "555001", trainerBID)
	if err != nil {
		t.Fatalf("SetTelegramActiveTrainer: %v", err)
	}
	if active.DisplayName != "Coach B" {
		t.Fatalf("active = %+v", active)
	}

	got, err := svc.GetTelegramActiveTrainer(ctx, "555001")
	if err != nil {
		t.Fatalf("GetTelegramActiveTrainer: %v", err)
	}
	if got.TrainerID != trainerBID {
		t.Fatalf("got = %+v", got)
	}
}

func inviteTokenFromURL(inviteURL string) string {
	return strings.TrimPrefix(strings.Split(inviteURL, "start=")[1], "inv_")
}

func TestTelegramClient_beforeProgramAssignment(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "tg2-trainer@test.com", pwHash, "Coach")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	invite, err := svc.CreateInvite(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
	if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token: token, TelegramUserID: "888002", DisplayName: "Client",
	}); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	today, err := svc.GetTelegramToday(ctx, "888002", nil)
	if err != nil {
		t.Fatalf("GetTelegramToday: %v", err)
	}
	if today.HasProgram {
		t.Fatalf("today = %+v", today)
	}

	programView, err := svc.GetTelegramProgram(ctx, "888002", nil)
	if err != nil {
		t.Fatalf("GetTelegramProgram: %v", err)
	}
	if programView.HasProgram {
		t.Fatalf("program = %+v", programView)
	}

	_, err = svc.GetTelegramProgram(ctx, "999999999", nil)
	if !errors.Is(err, trainerclient.ErrTelegramUserNotFound) {
		t.Fatalf("GetTelegramProgram unknown user err = %v", err)
	}

	list, err := svc.ListTelegramTrainers(ctx, "000000000")
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("ListTelegramTrainers unknown = %+v err=%v", list, err)
	}

	active, err := svc.GetTelegramActiveTrainer(ctx, "888002")
	if err != nil {
		t.Fatalf("GetTelegramActiveTrainer: %v", err)
	}
	if active.DisplayName == "" {
		t.Fatalf("active = %+v", active)
	}

	if _, err := progSvc.GetVersionDetail(ctx, uuid.New()); !errors.Is(err, program.ErrNotFound) {
		t.Fatalf("GetVersionDetail missing err = %v", err)
	}
}
