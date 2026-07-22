//go:build integration

package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/analytics"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/program"
)

type analyticsFixture struct {
	trainerUserID uuid.UUID
	trainerID     uuid.UUID
	clientUserID  uuid.UUID
	programID     uuid.UUID
	versionID     uuid.UUID
	cycleID       uuid.UUID
	dayKeys       []uuid.UUID // training days of the version, week order
}

// seedAnalyticsFixture creates a trainer with a linked client, a program with
// one frozen version (week 1: two training days; week 2: one training day and
// one empty day) and three completions:
//   - day 1 of week 1, current cycle, 1 day ago
//   - day 2 of week 1, current cycle, 10 days ago
//   - old-cycle completion, 40 days ago
func seedAnalyticsFixture(t *testing.T, pool *pgxpool.Pool, emailPrefix string) analyticsFixture {
	t.Helper()
	ctx := context.Background()
	q := sqlc.New(pool)

	authStore := auth.NewStore(pool)
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	trainerUserID, err := authStore.RegisterTrainerEmailPassword(ctx, emailPrefix+"-trainer@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register trainer: %v", err)
	}
	clientUserID, err := authStore.RegisterTrainerEmailPassword(ctx, emailPrefix+"-client@test.com", pwHash, "")
	if err != nil {
		t.Fatalf("register client: %v", err)
	}
	trainerIDPG, err := q.GetTrainerIDByUserID(ctx, pgconv.ToPGUUID(trainerUserID))
	if err != nil {
		t.Fatalf("trainer id: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
		VALUES ($1, $2, 'active')
	`, trainerIDPG, clientUserID); err != nil {
		t.Fatalf("link: %v", err)
	}

	draft, err := program.NewStore(pool).CreateDraft(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	now := time.Now().UTC()
	version, err := q.InsertProgramVersion(ctx, sqlc.InsertProgramVersionParams{
		ProgramID:          pgconv.ToPGUUID(draft.ID),
		VersionNumber:      1,
		PublishedAt:        now,
		PublishedBy:        pgconv.ToPGUUID(trainerUserID),
		Name:               "Analytics Program",
		NameRu:             "Аналитика",
		ContentFingerprint: emailPrefix + "-fp",
	})
	if err != nil {
		t.Fatalf("version: %v", err)
	}

	var dayKeys []uuid.UUID
	addDay := func(weekID pgtype.UUID, dayNumber int32, withBlock bool) uuid.UUID {
		key := uuid.New()
		day, err := q.InsertProgramVersionDay(ctx, sqlc.InsertProgramVersionDayParams{
			ProgramVersionID:     version.ID,
			ProgramVersionWeekID: weekID,
			DayNumber:            dayNumber,
			SortOrder:            dayNumber,
			DayKey:               pgconv.ToPGUUID(key),
		})
		if err != nil {
			t.Fatalf("version day: %v", err)
		}
		if withBlock {
			if _, err := q.InsertProgramVersionDayBlock(ctx, sqlc.InsertProgramVersionDayBlockParams{
				ProgramVersionWeekDayID: day.ID,
				BlockType:               "single",
				SortOrder:               1,
			}); err != nil {
				t.Fatalf("version block: %v", err)
			}
		}
		return key
	}

	week1, err := q.InsertProgramVersionWeek(ctx, sqlc.InsertProgramVersionWeekParams{
		ProgramVersionID: version.ID, WeekNumber: 1, SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("week1: %v", err)
	}
	week2, err := q.InsertProgramVersionWeek(ctx, sqlc.InsertProgramVersionWeekParams{
		ProgramVersionID: version.ID, WeekNumber: 2, SortOrder: 2,
	})
	if err != nil {
		t.Fatalf("week2: %v", err)
	}
	dayKeys = append(dayKeys, addDay(week1.ID, 1, true))
	dayKeys = append(dayKeys, addDay(week1.ID, 2, true))
	dayKeys = append(dayKeys, addDay(week2.ID, 1, true))
	addDay(week2.ID, 2, false) // empty day: not a training day

	cycleID := uuid.New()
	assignment, err := q.InsertProgramAssignment(ctx, sqlc.InsertProgramAssignmentParams{
		ProgramID:         pgconv.ToPGUUID(draft.ID),
		ProgramVersionID:  version.ID,
		TrainerID:         trainerIDPG,
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

	insertCompletion := func(cycle uuid.UUID, dayKey uuid.UUID, weekNumber, dayNumber int32, completedAt time.Time, result string) {
		if _, err := q.InsertClientWorkoutCompletion(ctx, sqlc.InsertClientWorkoutCompletionParams{
			ClientUserID:        pgconv.ToPGUUID(clientUserID),
			TrainerID:           trainerIDPG,
			CompletedAt:         completedAt,
			ProgramID:           pgconv.ToPGUUID(draft.ID),
			ProgramVersionID:    version.ID,
			ProgramAssignmentID: assignment.ID,
			CompletionCycleID:   pgconv.ToPGUUID(cycle),
			DayKey:              pgconv.ToPGUUID(dayKey),
			WeekNumber:          weekNumber,
			DayNumber:           dayNumber,
			ProgramName:         "Analytics Program",
			ProgramNameRu:       "Аналитика",
			DaySnapshot:         []byte(`{}`),
			ResultText:          result,
			Source:              "telegram",
		}); err != nil {
			t.Fatalf("completion: %v", err)
		}
	}

	insertCompletion(cycleID, dayKeys[0], 1, 1, now.Add(-24*time.Hour), "day one")
	insertCompletion(cycleID, dayKeys[1], 1, 2, now.Add(-10*24*time.Hour), "day two")
	oldCycle := uuid.New()
	insertCompletion(oldCycle, uuid.New(), 1, 1, now.Add(-40*24*time.Hour), "old cycle")

	return analyticsFixture{
		trainerUserID: trainerUserID,
		trainerID:     pgconv.FromPGUUID(trainerIDPG),
		clientUserID:  clientUserID,
		programID:     draft.ID,
		versionID:     pgconv.FromPGUUID(version.ID),
		cycleID:       cycleID,
		dayKeys:       dayKeys,
	}
}

func TestAnalyticsService_ClientAnalytics(t *testing.T) {
	pool := NewPool(t)
	fx := seedAnalyticsFixture(t, pool, "an-client")
	svc := analytics.NewService(pool, "test-jwt-secret-at-least-32-chars-long")
	ctx := context.Background()

	got, err := svc.ClientAnalytics(ctx, fx.trainerUserID, fx.clientUserID)
	if err != nil {
		t.Fatalf("ClientAnalytics: %v", err)
	}
	if got.Client.ClientUserID != fx.clientUserID || got.Client.Status != "active" {
		t.Fatalf("client = %+v", got.Client)
	}
	if got.Client.LastActiveAt == nil {
		t.Fatal("last_active_at should be set")
	}

	ca := got.CurrentAssignment
	if ca == nil {
		t.Fatal("current_assignment should be set")
	}
	if ca.ProgramID != fx.programID || ca.IsBehindLatest {
		t.Fatalf("assignment = %+v", ca)
	}
	if ca.Progress.CompletedDays != 2 || ca.Progress.TotalTrainingDays != 3 {
		t.Fatalf("progress = %+v", ca.Progress)
	}
	if ca.Progress.CompletionPercent != 66.7 {
		t.Fatalf("percent = %v", ca.Progress.CompletionPercent)
	}
	if len(ca.Progress.Weeks) != 2 {
		t.Fatalf("weeks = %+v", ca.Progress.Weeks)
	}
	w1, w2 := ca.Progress.Weeks[0], ca.Progress.Weeks[1]
	if w1.WeekNumber != 1 || w1.TotalDays != 2 || w1.CompletedDays != 2 {
		t.Fatalf("week1 = %+v", w1)
	}
	if w2.WeekNumber != 2 || w2.TotalDays != 1 || w2.CompletedDays != 0 {
		t.Fatalf("week2 = %+v", w2)
	}

	act := got.Activity
	if act.TotalCompletions != 3 || act.CompletionsLast7Days != 1 || act.CompletionsLast30Days != 2 {
		t.Fatalf("activity = %+v", act)
	}
	if act.FirstCompletedAt == nil || act.LastCompletedAt == nil {
		t.Fatal("first/last completed should be set")
	}
	if act.WeekStreak < 1 {
		t.Fatalf("week_streak = %d", act.WeekStreak)
	}
	if len(act.ByProgram) != 1 || act.ByProgram[0].TotalCompletions != 3 {
		t.Fatalf("by_program = %+v", act.ByProgram)
	}
	if act.ByProgram[0].ProgramID == nil || *act.ByProgram[0].ProgramID != fx.programID {
		t.Fatalf("by_program id = %+v", act.ByProgram[0].ProgramID)
	}

	// Foreign client → 404-style error.
	if _, err := svc.ClientAnalytics(ctx, fx.trainerUserID, uuid.New()); !errors.Is(err, analytics.ErrClientNotFound) {
		t.Fatalf("foreign client err = %v", err)
	}
	// Caller without a trainer profile → forbidden.
	if _, err := svc.ClientAnalytics(ctx, uuid.New(), fx.clientUserID); !errors.Is(err, analytics.ErrForbidden) {
		t.Fatalf("non-trainer err = %v", err)
	}
}

func TestAnalyticsService_ClientCompletions(t *testing.T) {
	pool := NewPool(t)
	fx := seedAnalyticsFixture(t, pool, "an-feed")
	svc := analytics.NewService(pool, "test-jwt-secret-at-least-32-chars-long")
	ctx := context.Background()

	params, err := analytics.ParseCompletionsParams("1", "10", "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.ClientCompletions(ctx, fx.trainerUserID, fx.clientUserID, params)
	if err != nil {
		t.Fatalf("ClientCompletions: %v", err)
	}
	if got.Pagination.Total != 3 || len(got.Items) != 3 {
		t.Fatalf("result = %+v", got.Pagination)
	}
	if !got.Items[0].CompletedAt.After(got.Items[2].CompletedAt) {
		t.Fatal("items should be sorted by completed_at desc")
	}
	if !got.Items[0].IsCurrentCycle || got.Items[2].IsCurrentCycle {
		t.Fatalf("cycle flags: %+v", got.Items)
	}
	if got.Items[0].ResultText != "day one" {
		t.Fatalf("newest = %+v", got.Items[0])
	}

	// from/to window keeps only the 10-days-ago completion.
	now := time.Now().UTC()
	from := now.Add(-20 * 24 * time.Hour).Format(time.RFC3339)
	to := now.Add(-5 * 24 * time.Hour).Format(time.RFC3339)
	params, err = analytics.ParseCompletionsParams("1", "10", from, to)
	if err != nil {
		t.Fatal(err)
	}
	got, err = svc.ClientCompletions(ctx, fx.trainerUserID, fx.clientUserID, params)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pagination.Total != 1 || got.Items[0].ResultText != "day two" {
		t.Fatalf("windowed = %+v", got.Items)
	}

	if _, err := svc.ClientCompletions(ctx, fx.trainerUserID, uuid.New(), params); !errors.Is(err, analytics.ErrClientNotFound) {
		t.Fatalf("foreign client err = %v", err)
	}
}

func TestAnalyticsService_ProgramsAnalytics(t *testing.T) {
	pool := NewPool(t)
	fx := seedAnalyticsFixture(t, pool, "an-programs")
	svc := analytics.NewService(pool, "test-jwt-secret-at-least-32-chars-long")
	ctx := context.Background()

	params, err := analytics.ParseProgramsParams("1", "10", "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.ProgramsAnalytics(ctx, fx.trainerUserID, params)
	if err != nil {
		t.Fatalf("ProgramsAnalytics: %v", err)
	}
	if got.Pagination.Total != 1 || len(got.Items) != 1 {
		t.Fatalf("result = %+v", got.Pagination)
	}
	item := got.Items[0]
	if item.ProgramID != fx.programID || item.ActiveClientsCount != 1 {
		t.Fatalf("item = %+v", item)
	}
	if item.TotalCompletions != 3 || item.CompletionsLast30Days != 2 {
		t.Fatalf("completions = %+v", item)
	}
	if item.AvgCompletionPercent == nil || *item.AvgCompletionPercent != 66.7 {
		t.Fatalf("avg percent = %v", item.AvgCompletionPercent)
	}
	if item.LastActivityAt == nil {
		t.Fatal("last_activity_at should be set")
	}

	if _, err := svc.ProgramsAnalytics(ctx, uuid.New(), params); !errors.Is(err, analytics.ErrForbidden) {
		t.Fatalf("non-trainer err = %v", err)
	}
}

func TestAnalyticsService_ProgramAnalytics(t *testing.T) {
	pool := NewPool(t)
	fx := seedAnalyticsFixture(t, pool, "an-detail")
	svc := analytics.NewService(pool, "test-jwt-secret-at-least-32-chars-long")
	ctx := context.Background()

	got, err := svc.ProgramAnalytics(ctx, fx.trainerUserID, fx.programID)
	if err != nil {
		t.Fatalf("ProgramAnalytics: %v", err)
	}
	if got.Program.ProgramID != fx.programID || got.Program.LatestVersionNumber != 1 {
		t.Fatalf("program = %+v", got.Program)
	}
	if got.Summary.ActiveClientsCount != 1 || got.Summary.TotalCompletions != 3 {
		t.Fatalf("summary = %+v", got.Summary)
	}
	if got.Summary.AvgCompletionPercent == nil || *got.Summary.AvgCompletionPercent != 66.7 {
		t.Fatalf("avg = %v", got.Summary.AvgCompletionPercent)
	}
	if len(got.Clients) != 1 {
		t.Fatalf("clients = %+v", got.Clients)
	}
	cl := got.Clients[0]
	if cl.ClientUserID != fx.clientUserID || cl.CompletedDays != 2 || cl.TotalTrainingDays != 3 {
		t.Fatalf("client row = %+v", cl)
	}
	if cl.CompletionPercent != 66.7 || cl.LastCompletedAt == nil || cl.IsBehindLatest {
		t.Fatalf("client row = %+v", cl)
	}
	// Only current-cycle completions count: both are in week 1.
	if len(got.Weeks) != 1 {
		t.Fatalf("weeks = %+v", got.Weeks)
	}
	if got.Weeks[0].WeekNumber != 1 || got.Weeks[0].CompletionsCount != 2 || got.Weeks[0].DistinctClientsCount != 1 {
		t.Fatalf("week stats = %+v", got.Weeks[0])
	}

	// Program owned by someone else → not found.
	other := seedAnalyticsFixture(t, pool, "an-detail-other")
	if _, err := svc.ProgramAnalytics(ctx, fx.trainerUserID, other.programID); !errors.Is(err, analytics.ErrProgramNotFound) {
		t.Fatalf("foreign program err = %v", err)
	}
	if _, err := svc.ProgramAnalytics(ctx, fx.trainerUserID, uuid.New()); !errors.Is(err, analytics.ErrProgramNotFound) {
		t.Fatalf("missing program err = %v", err)
	}
}

func TestAnalyticsService_ProgramWeekResults(t *testing.T) {
	pool := NewPool(t)
	fx := seedAnalyticsFixture(t, pool, "an-matrix")
	svc := analytics.NewService(pool, "test-jwt-secret-at-least-32-chars-long")
	ctx := context.Background()

	// Week 1: two training days, both submitted by the one client.
	week1, err := svc.ProgramWeekResults(ctx, fx.trainerUserID, fx.programID, 1)
	if err != nil {
		t.Fatalf("week1: %v", err)
	}
	if week1.ProgramID != fx.programID || week1.WeekNumber != 1 {
		t.Fatalf("header = %+v", week1)
	}
	if len(week1.Days) != 2 || week1.Days[0].DayNumber != 1 || week1.Days[1].DayNumber != 2 {
		t.Fatalf("days = %+v", week1.Days)
	}
	if len(week1.Clients) != 1 {
		t.Fatalf("clients = %d", len(week1.Clients))
	}
	cl := week1.Clients[0]
	if cl.ClientUserID != fx.clientUserID || cl.CompletedDays != 2 || cl.TotalDays != 2 {
		t.Fatalf("client = %+v", cl)
	}
	if len(cl.Days) != 2 {
		t.Fatalf("cells = %+v", cl.Days)
	}
	if cl.Days[0].Status != analytics.MatrixCellSubmitted || cl.Days[0].ResultText != "day one" {
		t.Fatalf("cell0 = %+v", cl.Days[0])
	}
	if cl.Days[0].CompletionID == nil || cl.Days[0].CompletedAt == nil {
		t.Fatalf("cell0 missing ids: %+v", cl.Days[0])
	}
	if cl.Days[0].Comments == nil {
		t.Fatal("comments must be non-nil slice")
	}
	if cl.Days[1].Status != analytics.MatrixCellSubmitted || cl.Days[1].ResultText != "day two" {
		t.Fatalf("cell1 = %+v", cl.Days[1])
	}
	if week1.Summary.TotalTrainingSlots != 2 || week1.Summary.SubmittedCount != 2 || week1.Summary.MissingCount != 0 {
		t.Fatalf("summary = %+v", week1.Summary)
	}
	if week1.Summary.CompletionPercent != 100 || week1.Summary.BehindClientsCount != 0 {
		t.Fatalf("summary = %+v", week1.Summary)
	}

	// Week 2: one training day, no submission → no_result.
	week2, err := svc.ProgramWeekResults(ctx, fx.trainerUserID, fx.programID, 2)
	if err != nil {
		t.Fatalf("week2: %v", err)
	}
	if len(week2.Days) != 1 || week2.Days[0].DayNumber != 1 {
		t.Fatalf("week2 days = %+v", week2.Days)
	}
	cell := week2.Clients[0].Days[0]
	if cell.Status != analytics.MatrixCellNoResult || cell.CompletionID != nil || cell.ResultText != "" {
		t.Fatalf("week2 cell = %+v", cell)
	}
	if week2.Summary.SubmittedCount != 0 || week2.Summary.MissingCount != 1 || week2.Summary.CompletionPercent != 0 {
		t.Fatalf("week2 summary = %+v", week2.Summary)
	}

	if _, err := svc.ProgramWeekResults(ctx, fx.trainerUserID, fx.programID, 99); !errors.Is(err, analytics.ErrWeekNotFound) {
		t.Fatalf("missing week err = %v", err)
	}
	if _, err := svc.ProgramWeekResults(ctx, fx.trainerUserID, fx.programID, 0); !errors.Is(err, analytics.ErrValidation) {
		t.Fatalf("week 0 err = %v", err)
	}
	if _, err := svc.ProgramWeekResults(ctx, fx.trainerUserID, uuid.New(), 1); !errors.Is(err, analytics.ErrProgramNotFound) {
		t.Fatalf("missing program err = %v", err)
	}
}
