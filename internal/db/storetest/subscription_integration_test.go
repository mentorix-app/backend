//go:build integration

package storetest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/admin"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
	"mentorix-backend/internal/trainerclient"
)

func TestSubscription_grantUsageAndQuota(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "sub-quota@test.com", pwHash, "Sub Trainer")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	subs := subscription.NewService(pool)
	sub, err := subs.ForUser(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}
	if sub == nil || sub.Plan != subscription.PlanFree {
		t.Fatalf("subscription = %+v, want free", sub)
	}
	if !sub.Permissions.CanCreateExercise {
		t.Fatal("expected free trainer to create exercises")
	}

	granted, err := subs.GrantAdminPlan(ctx, trainerUserID, subscription.PlanElite)
	if err != nil {
		t.Fatalf("GrantAdminPlan: %v", err)
	}
	if granted.Plan != subscription.PlanElite || granted.Source == nil || *granted.Source != subscription.SourceAdmin {
		t.Fatalf("granted = %+v, want elite/admin", granted)
	}
	if granted.Limits.Exercises != nil {
		t.Fatal("elite exercises must be unlimited")
	}

	revoked, err := subs.RevokeAdminPlan(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("RevokeAdminPlan: %v", err)
	}
	if revoked.Plan != subscription.PlanFree {
		t.Fatalf("after revoke plan = %s, want free", revoked.Plan)
	}

	if _, err := subs.GrantAdminPlan(ctx, trainerUserID, subscription.PlanAdvance); err != nil {
		t.Fatalf("GrantAdminPlan advance: %v", err)
	}
	freed, err := subs.GrantAdminPlan(ctx, trainerUserID, subscription.PlanFree)
	if err != nil {
		t.Fatalf("GrantAdminPlan free: %v", err)
	}
	if freed.Plan != subscription.PlanFree {
		t.Fatalf("free grant plan = %s", freed.Plan)
	}

	exStore := exercise.NewStore(pool)
	trainerID, err := exStore.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("TrainerIDForUser: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := exStore.Create(ctx, trainerUserID, &trainerID, exercise.UpsertInput{
			Name:        "Ex",
			NameRu:      "Упр",
			Type:        exercise.ExerciseTypeStrength,
			MuscleGroup: exercise.MuscleGroupLegs,
			Difficulty:  exercise.DifficultyBeginner,
		}); err != nil {
			t.Fatalf("create exercise %d: %v", i, err)
		}
	}
	_, err = exStore.Create(ctx, trainerUserID, &trainerID, exercise.UpsertInput{
		Name:        "Over",
		NameRu:      "Сверх",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	var qe *subscription.QuotaError
	if !errors.As(err, &qe) || qe.Resource != subscription.ResourceExercises {
		t.Fatalf("11th create error = %v, want QuotaError exercises", err)
	}

	progStore := program.NewStore(pool)
	for i := 0; i < 3; i++ {
		if _, err := progStore.CreateDraft(ctx, trainerUserID); err != nil {
			t.Fatalf("create draft %d: %v", i, err)
		}
	}
	_, err = progStore.CreateDraft(ctx, trainerUserID)
	if !errors.As(err, &qe) || qe.Resource != subscription.ResourcePrograms {
		t.Fatalf("4th draft error = %v, want QuotaError programs", err)
	}

	after, err := subs.ForUser(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("ForUser after usage: %v", err)
	}
	if after.Usage.Exercises != 10 || after.Usage.ActivePrograms != 3 {
		t.Fatalf("usage = %+v, want 10 exercises / 3 programs", after.Usage)
	}
	if after.Permissions.CanCreateExercise || after.Permissions.CanCreateProgram {
		t.Fatalf("permissions at limit must block create: %+v", after.Permissions)
	}
}

func TestSubscription_adminUserHasNilSubscription(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	adminID, err := authStore.RegisterTrainerEmailPassword(ctx, "sub-admin@test.com", pwHash, "Admin")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	promoteToAdminOnly(t, pool, adminID)

	subs := subscription.NewService(pool)
	sub, err := subs.ForUser(ctx, adminID)
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}
	if sub != nil {
		t.Fatalf("admin subscription = %+v, want nil", sub)
	}
}

func TestSubscription_grantInvalidAndMissingTrainer(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	subs := subscription.NewService(pool)

	_, err := subs.GrantAdminPlan(ctx, uuid.New(), subscription.Plan("platinum"))
	if !errors.Is(err, subscription.ErrInvalidPlan) {
		t.Fatalf("invalid plan err = %v, want ErrInvalidPlan", err)
	}

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	userID, err := authStore.RegisterTrainerEmailPassword(ctx, "sub-missing@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	promoteToAdminOnly(t, pool, userID)
	_, err = subs.GrantAdminPlan(ctx, userID, subscription.PlanElite)
	if !errors.Is(err, subscription.ErrTrainerNotFound) {
		t.Fatalf("err = %v, want ErrTrainerNotFound", err)
	}
}

func TestSubscription_clientInviteQuota(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "sub-clients@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	progSvc := program.NewService(pool)
	svc := trainerclient.NewService(pool, progSvc, trainerclient.InviteSettings{
		TelegramBotUsername: "mentorix_bot",
		InviteTTL:           7 * 24 * time.Hour,
	}, trainerclient.NewMemoryActiveTrainerStore(), nil)

	for i := 0; i < 3; i++ {
		invite, err := svc.CreateInvite(ctx, trainerUserID)
		if err != nil {
			t.Fatalf("CreateInvite %d: %v", i, err)
		}
		token := strings.TrimPrefix(strings.Split(invite.InviteURL, "start=")[1], "inv_")
		if _, err := svc.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
			Token:          token,
			TelegramUserID: fmt.Sprintf("sub-client-%d", i),
			DisplayName:    "Client",
		}); err != nil {
			t.Fatalf("AcceptInvite %d: %v", i, err)
		}
	}

	_, err = svc.CreateInvite(ctx, trainerUserID)
	var qe *subscription.QuotaError
	if !errors.As(err, &qe) || qe.Resource != subscription.ResourceClients {
		t.Fatalf("4th invite error = %v, want QuotaError clients", err)
	}
}

func TestSubscription_privateExerciseInProgram(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ownerID, err := authStore.RegisterTrainerEmailPassword(ctx, "sub-private-ex@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	otherID, err := authStore.RegisterTrainerEmailPassword(ctx, "sub-private-other@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register other: %v", err)
	}

	exStore := exercise.NewStore(pool)
	otherTrainerID, err := exStore.TrainerIDForUser(ctx, otherID)
	if err != nil {
		t.Fatalf("TrainerIDForUser: %v", err)
	}
	foreign, err := exStore.Create(ctx, otherID, &otherTrainerID, exercise.UpsertInput{
		Name:        "Foreign",
		NameRu:      "Чужое",
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create foreign: %v", err)
	}

	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, ownerID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	weekID := draft.Weeks[0].ID
	dayID := draft.Weeks[0].Days[0].ID
	sets, reps := "3", "10"
	_, err = createSingleBlock(ctx, svc, ownerID, draft.ID, weekID, dayID, program.DayExerciseInput{
		ExerciseID: foreign.ID,
		Sets:       &sets,
		Reps:       &reps,
	})
	if err == nil {
		t.Fatal("expected foreign private exercise to be rejected")
	}
}

func TestSubscription_checkerAndMutateQuota(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "sub-mutate@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exStore := exercise.NewStore(pool)
	trainerID, err := exStore.TrainerIDForUser(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("TrainerIDForUser: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := exStore.Create(ctx, trainerUserID, &trainerID, exercise.UpsertInput{
			Name:        "Ex",
			NameRu:      "Упр",
			Type:        exercise.ExerciseTypeStrength,
			MuscleGroup: exercise.MuscleGroupLegs,
			Difficulty:  exercise.DifficultyBeginner,
		}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	checker := subscription.NewChecker(pool)
	if err := checker.CheckQuota(ctx, trainerUserID, subscription.ResourceExercises, subscription.OpCreate); err == nil {
		t.Fatal("expected OpCreate blocked at limit")
	}
	if err := checker.CheckQuota(ctx, trainerUserID, subscription.ResourceExercises, subscription.OpMutate); err != nil {
		t.Fatalf("OpMutate at exact limit should pass: %v", err)
	}

	adminID, err := authStore.RegisterTrainerEmailPassword(ctx, "sub-checker-admin@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	promoteToAdminOnly(t, pool, adminID)
	if err := checker.CheckQuota(ctx, adminID, subscription.ResourceExercises, subscription.OpCreate); err != nil {
		t.Fatalf("admin without trainer profile must pass quota check: %v", err)
	}
}

func TestAdmin_grantAndRevokePlanHTTPLayer(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "admin-plan-target@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}

	adminSvc := admin.NewService(pool)
	sub, err := adminSvc.GrantPlan(ctx, trainerUserID, subscription.PlanElite)
	if err != nil {
		t.Fatalf("GrantPlan: %v", err)
	}
	if sub == nil || sub.Plan != subscription.PlanElite {
		t.Fatalf("sub = %+v", sub)
	}
	sub, err = adminSvc.RevokePlan(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("RevokePlan: %v", err)
	}
	if sub.Plan != subscription.PlanFree {
		t.Fatalf("plan = %s after revoke", sub.Plan)
	}
}

func TestProgram_deletePublishedClearsAssignments(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "del-pub@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	exStore := exercise.NewStore(pool)
	ex, err := exStore.Create(ctx, trainerUserID, nil, exercise.UpsertInput{
		Name: "Squat", NameRu: "Присед", Type: exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}
	svc := program.NewService(pool)
	draft, err := svc.Create(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	name := "To Delete"
	cat := program.CategoryMuscleGain
	diff := exercise.DifficultyBeginner
	if _, err := svc.Update(ctx, trainerUserID, draft.ID, program.UpdateInput{Name: &name, Category: &cat, Difficulty: &diff}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	sets, reps := "3", "8"
	if _, err := createSingleBlock(ctx, svc, trainerUserID, draft.ID, draft.Weeks[0].ID, draft.Weeks[0].Days[0].ID, program.DayExerciseInput{
		ExerciseID: ex.ID, Sets: &sets, Reps: &reps,
	}); err != nil {
		t.Fatalf("block: %v", err)
	}
	if _, err := svc.Publish(ctx, trainerUserID, draft.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := svc.Delete(ctx, trainerUserID, draft.ID); err != nil {
		t.Fatalf("Delete published: %v", err)
	}
}

func TestSubscription_republishBlockedWhenAtProgramLimit(t *testing.T) {
	pool := NewPool(t)
	ctx := context.Background()
	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, "republish-quota@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	exStore := exercise.NewStore(pool)
	ex, err := exStore.Create(ctx, trainerUserID, nil, exercise.UpsertInput{
		Name: "Squat", NameRu: "Присед", Type: exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs, Difficulty: exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("exercise: %v", err)
	}
	svc := program.NewService(pool)
	publishOne := func(emailSuffix string) uuid.UUID {
		t.Helper()
		d, err := svc.Create(ctx, trainerUserID)
		if err != nil {
			t.Fatalf("Create %s: %v", emailSuffix, err)
		}
		name := "P-" + emailSuffix
		cat := program.CategoryMuscleGain
		diff := exercise.DifficultyBeginner
		if _, err := svc.Update(ctx, trainerUserID, d.ID, program.UpdateInput{Name: &name, Category: &cat, Difficulty: &diff}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		sets, reps := "3", "8"
		if _, err := createSingleBlock(ctx, svc, trainerUserID, d.ID, d.Weeks[0].ID, d.Weeks[0].Days[0].ID, program.DayExerciseInput{
			ExerciseID: ex.ID, Sets: &sets, Reps: &reps,
		}); err != nil {
			t.Fatalf("block: %v", err)
		}
		if _, err := svc.Publish(ctx, trainerUserID, d.ID); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		return d.ID
	}
	first := publishOne("1")
	_ = publishOne("2")
	_ = publishOne("3")
	if _, err := svc.Archive(ctx, trainerUserID, first); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	// Fill the freed slot so republish would exceed free limit of 3.
	_ = publishOne("4")
	_, err = svc.Publish(ctx, trainerUserID, first)
	var qe *subscription.QuotaError
	if !errors.As(err, &qe) || qe.Resource != subscription.ResourcePrograms {
		t.Fatalf("republish error = %v, want programs QuotaError", err)
	}
}
