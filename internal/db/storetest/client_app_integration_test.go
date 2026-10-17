//go:build integration

package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/analytics"
	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
	"mentorix-backend/internal/workoutcompletion"
)

// appFixture is one trainer with a published program. Day 1 has three blocks: a shared
// block, a block for clientY only, and a block for both clients. Day 2 has one block
// that only clientY may see. Day 3 and later are rest days. clientX and clientY are
// linked and both assigned the program.
type appFixture struct {
	env           appInviteEnv
	trainerUserID uuid.UUID
	trainerID     uuid.UUID
	programID     uuid.UUID
	blocks        []program.DayBlock
	clientX       uuid.UUID
	clientY       uuid.UUID
}

func newAppFixture(t *testing.T, prefix string) appFixture {
	t.Helper()
	ctx := context.Background()
	env := newAppInviteEnv(t)
	trainerUserID, exerciseID := seedTrainerAndExercise(t, env.pool, prefix)
	store := program.NewStore(env.pool)
	detail, err := store.CreateDraft(ctx, trainerUserID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	week := detail.Weeks[0]
	for i, day := range []program.Day{week.Days[0], week.Days[0], week.Days[0], week.Days[1]} {
		detail, err = createSingleBlockStore(ctx, store, trainerUserID, detail.ID, week.ID, day.ID,
			program.DayExerciseInput{ExerciseID: exerciseID})
		if err != nil {
			t.Fatalf("createSingleBlockStore %d: %v", i, err)
		}
	}
	published, err := store.PublishFromDraft(ctx, detail.ID, trainerUserID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	programID := published.ID
	blocks := published.Weeks[0].Days[0].Blocks
	clientX, trainerID := seedAssignedClient(t, env.pool, trainerUserID, prefix+"-x")
	clientY, _ := seedAssignedClient(t, env.pool, trainerUserID, prefix+"-y")
	if _, err := env.pool.Exec(ctx, `UPDATE mentorix.users SET display_name = 'Coach Max' WHERE id = $1`, trainerUserID); err != nil {
		t.Fatalf("name the trainer: %v", err)
	}
	for _, c := range []uuid.UUID{clientX, clientY} {
		if _, err := store.SetClientProgramAssignment(ctx, trainerUserID, trainerID, c, &programID); err != nil {
			t.Fatalf("assign %v: %v", c, err)
		}
	}
	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, week.ID, blocks[1].ID, []uuid.UUID{clientY}); err != nil {
		t.Fatalf("restrict block 1: %v", err)
	}
	if _, err := store.SetBlockClients(ctx, trainerUserID, programID, week.ID, blocks[2].ID, []uuid.UUID{clientX, clientY}); err != nil {
		t.Fatalf("restrict block 2: %v", err)
	}
	// SetBlockClients refuses to hide a day's last shared block, so the day whose only
	// block is for clientY alone gets its rule straight from the table.
	if _, err := env.pool.Exec(ctx, `
INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id) VALUES ($1, $2, $3)`,
		programID, published.Weeks[0].Days[1].Blocks[0].BlockKey, clientY); err != nil {
		t.Fatalf("hide the block of day 2: %v", err)
	}
	return appFixture{
		env: env, trainerUserID: trainerUserID, trainerID: trainerID, programID: programID,
		blocks: blocks, clientX: clientX, clientY: clientY,
	}
}

func (f appFixture) view(t *testing.T, client uuid.UUID) trainerclient.ClientProgramResponse {
	t.Helper()
	got, err := f.env.svc.GetClientProgram(context.Background(), client, f.trainerID)
	if err != nil {
		t.Fatalf("GetClientProgram: %v", err)
	}
	return got
}

// request builds the mark request an app would send for the program it just read.
func requestFor(v trainerclient.ClientProgramResponse, dayKey uuid.UUID, text string) trainerclient.ClientCompletionRequest {
	return trainerclient.ClientCompletionRequest{
		ProgramAssignmentID: v.Assignment.AssignmentID,
		ProgramVersionID:    v.Assignment.ProgramVersionID,
		CompletionCycleID:   v.Assignment.CompletionCycleID,
		DayKey:              dayKey,
		ResultText:          text,
	}
}

func firstDayKey(t *testing.T, v trainerclient.ClientProgramResponse) uuid.UUID {
	t.Helper()
	if v.Program == nil || len(v.Program.Weeks) == 0 || len(v.Program.Weeks[0].Days) == 0 {
		t.Fatalf("program view has no days: %+v", v)
	}
	return v.Program.Weeks[0].Days[0].DayKey
}

func (f appFixture) completionRows(t *testing.T, client uuid.UUID) int {
	t.Helper()
	return countRows(t, f.env.pool, `SELECT count(*) FROM mentorix.client_workout_completions WHERE client_user_id = $1`, client)
}

// publishSecondProgram publishes another program of the same trainer and returns its id.
func (f appFixture) publishSecondProgram(t *testing.T, prefix string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	store := program.NewStore(f.env.pool)
	exerciseID := mustExercise(t, f.env, f.trainerUserID, prefix)
	draft, err := store.CreateDraft(ctx, f.trainerUserID)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	detail, err := createSingleBlockStore(ctx, store, f.trainerUserID, draft.ID, draft.Weeks[0].ID, draft.Weeks[0].Days[0].ID,
		program.DayExerciseInput{ExerciseID: exerciseID})
	if err != nil {
		t.Fatalf("createSingleBlockStore: %v", err)
	}
	published, err := store.PublishFromDraft(ctx, detail.ID, f.trainerUserID, detail)
	if err != nil {
		t.Fatalf("PublishFromDraft: %v", err)
	}
	return published.ID
}

func mustExercise(t *testing.T, env appInviteEnv, trainerUserID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	ex, err := exercise.NewStore(env.pool).Create(context.Background(), trainerUserID, nil, exercise.UpsertInput{
		Name:        name,
		NameRu:      name,
		Type:        exercise.ExerciseTypeStrength,
		MuscleGroup: exercise.MuscleGroupLegs,
		Difficulty:  exercise.DifficultyBeginner,
	})
	if err != nil {
		t.Fatalf("create exercise %s: %v", name, err)
	}
	return ex.ID
}

func TestMigration_completionSourceCheckAllowsApp(t *testing.T) {
	f := newAppFixture(t, "ca-mig")
	ctx := context.Background()

	var def string
	if err := f.env.pool.QueryRow(ctx, `
SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'client_workout_completions_source_check'`).Scan(&def); err != nil {
		t.Fatalf("read constraint: %v", err)
	}
	if !strings.Contains(def, "'telegram'") || !strings.Contains(def, "'app'") {
		t.Fatalf("constraint = %s, want it to allow telegram and app", def)
	}

	v := f.view(t, f.clientX)
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(v, firstDayKey(t, v), "ok")); err != nil {
		t.Fatalf("CompleteClientDay: %v", err)
	}
	_, err := f.env.pool.Exec(ctx, `UPDATE mentorix.client_workout_completions SET source = 'web' WHERE client_user_id = $1`, f.clientX)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("setting source web: err = %v, want a check violation", err)
	}
}

func TestClientApp_programView_requiresAnActiveLinkToThatTrainer(t *testing.T) {
	f := newAppFixture(t, "ca-link")
	ctx := context.Background()
	stranger := newClientWithIdentity(t, f.env.auth, auth.ProviderGoogle, "g-ca-stranger", "ca-stranger@test.com")
	otherTrainer := f.env.trainer(t, "ca-link-other@test.com", "Other")
	otherTrainerID := trainerIDOf(t, f.env, otherTrainer)

	for name, tt := range map[string]struct{ client, trainer uuid.UUID }{
		"a client with no link":                      {stranger, f.trainerID},
		"a linked client asking for another trainer": {f.clientX, otherTrainerID},
		"an unknown trainer id":                      {f.clientX, uuid.New()},
		"an unknown user":                            {uuid.New(), f.trainerID},
	} {
		_, err := f.env.svc.GetClientProgram(ctx, tt.client, tt.trainer)
		if !errors.Is(err, program.ErrClientNotLinked) {
			t.Errorf("%s: err = %v, want ErrClientNotLinked", name, err)
		}
	}
}

func TestClientApp_programView_blockedClientIsRefused(t *testing.T) {
	f := newAppFixture(t, "ca-blocked")
	ctx := context.Background()
	if _, err := f.env.pool.Exec(ctx, `UPDATE mentorix.trainer_clients SET status = 'blocked' WHERE client_user_id = $1`, f.clientX); err != nil {
		t.Fatalf("block clientX: %v", err)
	}

	if _, err := f.env.svc.GetClientProgram(ctx, f.clientX, f.trainerID); !errors.Is(err, program.ErrClientBlocked) {
		t.Fatalf("blocked client: err = %v, want ErrClientBlocked", err)
	}
	if _, err := f.env.svc.GetClientProgram(ctx, f.clientY, f.trainerID); err != nil {
		t.Fatalf("the other client is still fine: %v", err)
	}
}

func TestClientApp_programView_noAssignment(t *testing.T) {
	f := newAppFixture(t, "ca-none")
	clientZ, _ := seedAssignedClient(t, f.env.pool, f.trainerUserID, "ca-none-z")

	got := f.view(t, clientZ)

	if got.Assignment != nil || got.Program != nil {
		t.Fatalf("assignment %+v program %+v, want both nil", got.Assignment, got.Program)
	}
	if got.TrainerID != f.trainerID || got.TrainerDisplayName != "Coach Max" {
		t.Fatalf("trainer = %v %q", got.TrainerID, got.TrainerDisplayName)
	}
	if got.CompletedDays == nil || len(got.CompletedDays) != 0 {
		t.Fatalf("completed_days = %#v, want a non-nil empty slice", got.CompletedDays)
	}
}

func TestClientApp_programView_showsOnlyTheBlocksTheClientMaySee(t *testing.T) {
	f := newAppFixture(t, "ca-hidden")

	x := f.view(t, f.clientX)
	y := f.view(t, f.clientY)

	// Block rows are regenerated when a version is frozen; block_key is the stable id.
	keysOf := func(v trainerclient.ClientProgramResponse) []uuid.UUID {
		var keys []uuid.UUID
		for _, b := range v.Program.Weeks[0].Days[0].Blocks {
			keys = append(keys, b.BlockKey)
		}
		return keys
	}
	if got := keysOf(x); len(got) != 2 || got[0] != f.blocks[0].BlockKey || got[1] != f.blocks[2].BlockKey {
		t.Fatalf("clientX sees blocks %v, want the block keys %v and %v", got, f.blocks[0].BlockKey, f.blocks[2].BlockKey)
	}
	if got := keysOf(y); len(got) != 3 {
		t.Fatalf("clientY sees %d blocks, want 3", len(got))
	}
	if x.Assignment == nil || x.Assignment.ProgramID != f.programID {
		t.Fatalf("assignment = %+v", x.Assignment)
	}
}

// A block shared by two clients carries both ids in its rule. The app must not
// learn who else the trainer put on the program.
func TestClientApp_programView_doesNotRevealOtherClients(t *testing.T) {
	f := newAppFixture(t, "ca-privacy")

	raw, err := json.Marshal(f.view(t, f.clientX))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(raw), f.clientY.String()) {
		t.Fatalf("the program answer for clientX names clientY: %s", raw)
	}
	blocks := f.view(t, f.clientX).Program.Weeks[0].Days[0].Blocks
	if len(blocks[0].ClientUserIDs) != 0 || len(blocks[1].ClientUserIDs) != 1 || blocks[1].ClientUserIDs[0] != f.clientX {
		t.Fatalf("client_user_ids = %v / %v, want [] for the shared block and only the caller for the shared-with-me block",
			blocks[0].ClientUserIDs, blocks[1].ClientUserIDs)
	}
}

func TestClientApp_markDone_writesAnAppRowWithTheDaySnapshot(t *testing.T) {
	f := newAppFixture(t, "ca-mark")
	ctx := context.Background()
	v := f.view(t, f.clientX)
	dayKey := firstDayKey(t, v)

	got, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(v, dayKey, "  felt strong  "))

	if err != nil {
		t.Fatalf("CompleteClientDay: %v", err)
	}
	if got.ID == uuid.Nil || got.DayKey != dayKey || got.WeekNumber != 1 || got.DayNumber != 1 || got.ResultText != "felt strong" || got.CompletedAt.IsZero() {
		t.Fatalf("answer = %+v", got)
	}
	var source, text string
	var assignmentID, versionID, programID, trainerID, client uuid.UUID
	var blocks int
	err = f.env.pool.QueryRow(ctx, `
SELECT source, result_text, program_assignment_id, program_version_id, program_id, trainer_id, client_user_id,
       jsonb_array_length(day_snapshot->'blocks')
FROM mentorix.client_workout_completions WHERE id = $1`, got.ID).
		Scan(&source, &text, &assignmentID, &versionID, &programID, &trainerID, &client, &blocks)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if source != "app" || text != "felt strong" {
		t.Fatalf("row source %q text %q", source, text)
	}
	if assignmentID != v.Assignment.AssignmentID || versionID != v.Assignment.ProgramVersionID || programID != f.programID ||
		trainerID != f.trainerID || client != f.clientX {
		t.Fatalf("row ids: assignment %v version %v program %v trainer %v client %v", assignmentID, versionID, programID, trainerID, client)
	}
	if blocks != 2 {
		t.Fatalf("snapshot holds %d blocks, want the 2 clientX may see", blocks)
	}

	after := f.view(t, f.clientX)
	if len(after.CompletedDays) != 1 || after.CompletedDays[0].DayKey != dayKey {
		t.Fatalf("completed_days = %v, want [%v]", after.CompletedDays, dayKey)
	}
}

func TestClientApp_markDone_secondMarkIsAlreadyCompleted(t *testing.T) {
	f := newAppFixture(t, "ca-twice")
	ctx := context.Background()
	v := f.view(t, f.clientX)
	req := requestFor(v, firstDayKey(t, v), "first")
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, req); err != nil {
		t.Fatalf("first mark: %v", err)
	}

	req.ResultText = "second"
	_, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, req)

	if !errors.Is(err, workoutcompletion.ErrAlreadyCompleted) {
		t.Fatalf("second mark err = %v, want ErrAlreadyCompleted", err)
	}
	if n := f.completionRows(t, f.clientX); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func TestClientApp_markDone_reassignmentBetweenReadAndWrite(t *testing.T) {
	f := newAppFixture(t, "ca-reassign")
	ctx := context.Background()
	v := f.view(t, f.clientX)
	oldReq := requestFor(v, firstDayKey(t, v), "late")
	second := f.publishSecondProgram(t, "ca-reassign")
	if _, err := program.NewStore(f.env.pool).SetClientProgramAssignment(ctx, f.trainerUserID, f.trainerID, f.clientX, &second); err != nil {
		t.Fatalf("reassign: %v", err)
	}

	_, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, oldReq)

	if !errors.Is(err, trainerclient.ErrProgramChanged) {
		t.Fatalf("err = %v, want ErrProgramChanged", err)
	}
	if n := f.completionRows(t, f.clientX); n != 0 {
		t.Fatalf("rows = %d, want none written to the new cycle", n)
	}

	fresh := f.view(t, f.clientX)
	if fresh.Assignment.ProgramID != second || len(fresh.CompletedDays) != 0 {
		t.Fatalf("fresh view = %+v", fresh)
	}
	// A day key of the program the client left is not a day of the new one.
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(fresh, oldReq.DayKey, "ok")); !errors.Is(err, trainerclient.ErrDayNotFound) {
		t.Fatalf("old day key on the new program: err = %v, want ErrDayNotFound", err)
	}
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(fresh, firstDayKey(t, fresh), "ok")); err != nil {
		t.Fatalf("mark on the new program: %v", err)
	}
}

func TestClientApp_markDone_unknownDays(t *testing.T) {
	f := newAppFixture(t, "ca-day")
	ctx := context.Background()
	v := f.view(t, f.clientX)
	restDay := v.Program.Weeks[0].Days[2].DayKey
	otherProgram := f.publishSecondProgram(t, "ca-day")
	otherView := func() trainerclient.ClientProgramResponse {
		clientZ, _ := seedAssignedClient(t, f.env.pool, f.trainerUserID, "ca-day-z")
		if _, err := program.NewStore(f.env.pool).SetClientProgramAssignment(ctx, f.trainerUserID, f.trainerID, clientZ, &otherProgram); err != nil {
			t.Fatalf("assign other program: %v", err)
		}
		return f.view(t, clientZ)
	}()

	for name, key := range map[string]uuid.UUID{
		"a random key":                      uuid.New(),
		"a day of another program":          firstDayKey(t, otherView),
		"a day with no blocks (a rest day)": restDay,
	} {
		_, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(v, key, "ok"))
		if !errors.Is(err, trainerclient.ErrDayNotFound) {
			t.Errorf("%s: err = %v, want ErrDayNotFound", name, err)
		}
	}
	if n := f.completionRows(t, f.clientX); n != 0 {
		t.Fatalf("rows = %d, want none", n)
	}
}

func TestClientApp_markDone_noAssignmentIsProgramChanged(t *testing.T) {
	f := newAppFixture(t, "ca-noassign")
	v := f.view(t, f.clientX)
	clientZ, _ := seedAssignedClient(t, f.env.pool, f.trainerUserID, "ca-noassign-z")

	_, err := f.env.svc.CompleteClientDay(context.Background(), clientZ, f.trainerID, requestFor(v, firstDayKey(t, v), "ok"))

	if !errors.Is(err, trainerclient.ErrProgramChanged) {
		t.Fatalf("err = %v, want ErrProgramChanged", err)
	}
}

func TestClientApp_anotherClientsDataIsNotVisibleOrUsable(t *testing.T) {
	f := newAppFixture(t, "ca-iso")
	ctx := context.Background()
	x := f.view(t, f.clientX)
	dayKey := firstDayKey(t, x)
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(x, dayKey, "x did it")); err != nil {
		t.Fatalf("clientX mark: %v", err)
	}

	y := f.view(t, f.clientY)
	if len(y.CompletedDays) != 0 {
		t.Fatalf("clientY sees clientX's done days: %v", y.CompletedDays)
	}
	// clientX's assignment id does not let clientY write.
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientY, f.trainerID, requestFor(x, dayKey, "y says so")); !errors.Is(err, trainerclient.ErrProgramChanged) {
		t.Fatalf("clientY with clientX's assignment: err = %v, want ErrProgramChanged", err)
	}
	if n := f.completionRows(t, f.clientY); n != 0 {
		t.Fatalf("clientY rows = %d, want none", n)
	}
	// clientY marks their own copy of the same day independently.
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientY, f.trainerID, requestFor(y, dayKey, "y did it")); err != nil {
		t.Fatalf("clientY own mark: %v", err)
	}
}

func TestClientApp_markDone_refusesWithoutAnActiveLink(t *testing.T) {
	f := newAppFixture(t, "ca-markgate")
	ctx := context.Background()
	v := f.view(t, f.clientX)
	req := requestFor(v, firstDayKey(t, v), "ok")
	stranger := newClientWithIdentity(t, f.env.auth, auth.ProviderGoogle, "g-ca-markgate", "ca-markgate@test.com")

	if _, err := f.env.svc.CompleteClientDay(ctx, stranger, f.trainerID, req); !errors.Is(err, program.ErrClientNotLinked) {
		t.Errorf("stranger: err = %v, want ErrClientNotLinked", err)
	}
	if _, err := f.env.pool.Exec(ctx, `UPDATE mentorix.trainer_clients SET status = 'blocked' WHERE client_user_id = $1`, f.clientX); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, req); !errors.Is(err, program.ErrClientBlocked) {
		t.Errorf("blocked: err = %v, want ErrClientBlocked", err)
	}
	if n := countRows(t, f.env.pool, `SELECT count(*) FROM mentorix.client_workout_completions`); n != 0 {
		t.Fatalf("rows = %d, want none", n)
	}
}

func TestClientApp_markDone_refusesBadText(t *testing.T) {
	f := newAppFixture(t, "ca-text")
	ctx := context.Background()
	v := f.view(t, f.clientX)
	dayKey := firstDayKey(t, v)

	for name, text := range map[string]string{"empty": "", "blank": " \n\t", "too long": strings.Repeat("я", workoutcompletion.MaxResultTextLen+1)} {
		if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(v, dayKey, text)); !errors.Is(err, workoutcompletion.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
	if n := f.completionRows(t, f.clientX); n != 0 {
		t.Fatalf("rows = %d, want none", n)
	}
	exact := strings.Repeat("я", workoutcompletion.MaxResultTextLen)
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(v, dayKey, exact)); err != nil {
		t.Fatalf("text of exactly the limit: %v", err)
	}
}

const clientAppSecret = "test-jwt-secret-at-least-32-chars-long"

func clientAppEcho(f appFixture) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	trainerclient.NewHandlers(f.env.svc, f.env.pool, clientAppSecret).Mount(e)
	analytics.NewHandlers(analytics.NewService(f.env.pool, clientAppSecret), f.env.pool, clientAppSecret).Mount(e)
	return e
}

func callAs(t *testing.T, e *echo.Echo, user uuid.UUID, method, path, body string) (int, string) {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    auth.Issuer,
		Subject:   user.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte(clientAppSecret))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signed)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestClientApp_overHTTP(t *testing.T) {
	f := newAppFixture(t, "ca-http")
	e := clientAppEcho(f)
	base := "/client/trainers/" + f.trainerID.String()

	// Read the program.
	code, body := callAs(t, e, f.clientX, http.MethodGet, base+"/program", "")
	if code != http.StatusOK {
		t.Fatalf("GET program: %d %s", code, body)
	}
	var view struct {
		Assignment struct {
			AssignmentID      uuid.UUID `json:"assignment_id"`
			ProgramVersionID  uuid.UUID `json:"program_version_id"`
			CompletionCycleID uuid.UUID `json:"completion_cycle_id"`
		} `json:"assignment"`
		Program struct {
			Weeks []struct {
				Days []struct {
					DayKey uuid.UUID `json:"day_key"`
				} `json:"days"`
			} `json:"weeks"`
		} `json:"program"`
		CompletedDays []struct {
			DayKey uuid.UUID `json:"day_key"`
		} `json:"completed_days"`
	}
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode: %v; %s", err, body)
	}
	dayKey := view.Program.Weeks[0].Days[0].DayKey
	if dayKey == uuid.Nil {
		t.Fatalf("the program answer must carry day_key on every day: %s", body)
	}
	if !strings.Contains(body, `"completed_days":[]`) {
		t.Fatalf("body lacks an empty completed_days array: %s", body)
	}

	// Mark the day.
	payload, _ := json.Marshal(map[string]any{
		"program_assignment_id": view.Assignment.AssignmentID, "program_version_id": view.Assignment.ProgramVersionID,
		"completion_cycle_id": view.Assignment.CompletionCycleID, "day_key": dayKey, "result_text": "heavy day",
	})
	code, body = callAs(t, e, f.clientX, http.MethodPost, base+"/completions", string(payload))
	if code != http.StatusCreated {
		t.Fatalf("POST completions: %d %s", code, body)
	}
	if !strings.Contains(body, `"result_text":"heavy day"`) || !strings.Contains(body, `"week_number":1`) || !strings.Contains(body, `"day_number":1`) {
		t.Fatalf("created body = %s", body)
	}

	// The checkmark shows and the second mark is refused.
	code, body = callAs(t, e, f.clientX, http.MethodGet, base+"/program", "")
	if code != http.StatusOK || !strings.Contains(body, `"completed_days":[{"day_key":"`+dayKey.String()+`"}]`) {
		t.Fatalf("GET program after mark: %d %s", code, body)
	}
	code, body = callAs(t, e, f.clientX, http.MethodPost, base+"/completions", string(payload))
	if code != http.StatusConflict || !strings.Contains(body, `"code":"workout_already_completed"`) {
		t.Fatalf("second POST: %d %s", code, body)
	}

	// Another client of the same trainer, and a stranger.
	if code, body := callAs(t, e, f.clientY, http.MethodPost, base+"/completions", string(payload)); code != http.StatusConflict || !strings.Contains(body, `"code":"program_changed"`) {
		t.Fatalf("clientY with clientX's ids: %d %s", code, body)
	}
	stranger := newClientWithIdentity(t, f.env.auth, auth.ProviderGoogle, "g-ca-http", "ca-http@test.com")
	if code, body := callAs(t, e, stranger, http.MethodGet, base+"/program", ""); code != http.StatusForbidden || !strings.Contains(body, `"code":"client_not_linked"`) {
		t.Fatalf("stranger program: %d %s", code, body)
	}
	if code, body := callAs(t, e, stranger, http.MethodPost, base+"/completions", string(payload)); code != http.StatusForbidden || !strings.Contains(body, `"code":"client_not_linked"`) {
		t.Fatalf("stranger completions: %d %s", code, body)
	}
}

func TestClientApp_analyticsOverHTTP(t *testing.T) {
	f := newAppFixture(t, "ca-stats")
	ctx := context.Background()
	e := clientAppEcho(f)
	base := "/client/trainers/" + f.trainerID.String()
	v := f.view(t, f.clientX)
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(v, firstDayKey(t, v), "stats please")); err != nil {
		t.Fatalf("mark: %v", err)
	}

	code, body := callAs(t, e, f.clientX, http.MethodGet, base+"/analytics", "")
	if code != http.StatusOK {
		t.Fatalf("GET analytics: %d %s", code, body)
	}
	if !strings.Contains(body, `"expires_at":null`) {
		t.Fatalf("the token route has no link to expire, expires_at must be null: %s", body)
	}
	var got analytics.ClientSelfAnalytics
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v; %s", err, body)
	}
	want, err := analytics.NewService(f.env.pool, clientAppSecret).ClientSelfAnalytics(ctx, f.clientX, f.trainerID)
	if err != nil {
		t.Fatalf("ClientSelfAnalytics: %v", err)
	}
	if got.Client != want.Client || got.Trainer != want.Trainer || len(got.RecentCompletions) != 1 || got.RecentCompletions[0].ResultText != "stats please" ||
		got.Activity.TotalCompletions != want.Activity.TotalCompletions {
		t.Fatalf("token route body differs from the page's service answer:\n got  %+v\n want %+v", got, want)
	}

	// Not linked answers as the page does; so does a blocked client.
	stranger := newClientWithIdentity(t, f.env.auth, auth.ProviderGoogle, "g-ca-stats", "ca-stats@test.com")
	if code, body := callAs(t, e, stranger, http.MethodGet, base+"/analytics", ""); code != http.StatusNotFound || !strings.Contains(body, `"code":"client_not_found"`) {
		t.Fatalf("stranger: %d %s", code, body)
	}
	if _, err := f.env.pool.Exec(ctx, `UPDATE mentorix.trainer_clients SET status = 'blocked' WHERE client_user_id = $1`, f.clientX); err != nil {
		t.Fatal(err)
	}
	if code, body := callAs(t, e, f.clientX, http.MethodGet, base+"/analytics", ""); code != http.StatusNotFound || !strings.Contains(body, `"code":"client_not_found"`) {
		t.Fatalf("blocked: %d %s", code, body)
	}
}

// A to B to A keeps the assignment row and brings back the same version, so the two
// ids alone cannot tell the third cycle from the first. A request from the first
// cycle must not land in the third.
func TestClientApp_markDone_staleRequestAfterAToBToA(t *testing.T) {
	f := newAppFixture(t, "ca-atba")
	ctx := context.Background()
	v := f.view(t, f.clientX)
	oldReq := requestFor(v, firstDayKey(t, v), "from the first cycle")
	second := f.publishSecondProgram(t, "ca-atba")
	store := program.NewStore(f.env.pool)
	if _, err := store.SetClientProgramAssignment(ctx, f.trainerUserID, f.trainerID, f.clientX, &second); err != nil {
		t.Fatalf("assign B: %v", err)
	}
	if _, err := store.SetClientProgramAssignment(ctx, f.trainerUserID, f.trainerID, f.clientX, &f.programID); err != nil {
		t.Fatalf("assign A again: %v", err)
	}
	again := f.view(t, f.clientX)
	if again.Assignment.AssignmentID != v.Assignment.AssignmentID || again.Assignment.ProgramVersionID != v.Assignment.ProgramVersionID {
		t.Fatalf("test premise broken: the assignment and version ids should come back, got %+v and %+v", again.Assignment, v.Assignment)
	}

	_, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, oldReq)

	if !errors.Is(err, trainerclient.ErrProgramChanged) {
		t.Fatalf("err = %v, want ErrProgramChanged", err)
	}
	if n := f.completionRows(t, f.clientX); n != 0 {
		t.Fatalf("rows = %d, want none written to the third cycle", n)
	}
}

// The conditional insert is the guard that holds even when two requests race past the
// service's pre-check: it writes only while the assignment row still has the cycle,
// version and client the mark names.
func TestClientApp_conditionalInsert_refusesAStaleCycle(t *testing.T) {
	f := newAppFixture(t, "ca-cond")
	ctx := context.Background()
	v := f.view(t, f.clientX)
	dayKey := firstDayKey(t, v)
	store := workoutcompletion.NewStore(f.env.pool)
	completion := func(a trainerclient.ClientProgramResponse, cycle uuid.UUID) workoutcompletion.Completion {
		assignment, version := a.Assignment.AssignmentID, a.Assignment.ProgramVersionID
		return workoutcompletion.Completion{
			ClientUserID: f.clientX, TrainerID: f.trainerID, CompletedAt: time.Now().UTC(),
			ProgramAssignmentID: &assignment, ProgramVersionID: &version, CompletionCycleID: cycle,
			DayKey: dayKey, WeekNumber: 1, DayNumber: 1, ProgramName: "p", ProgramNameRu: "p",
			DaySnapshot: []byte(`{"blocks":[]}`), ResultText: "ok", Source: workoutcompletion.SourceApp,
		}
	}

	// A cycle the assignment never had, a version it does not point to and a client it does not belong to.
	if _, err := store.InsertForAssignment(ctx, completion(v, uuid.New())); !errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
		t.Fatalf("unknown cycle: err = %v, want ErrAssignmentChanged", err)
	}
	other := completion(v, v.Assignment.CompletionCycleID)
	otherVersion := uuid.New()
	other.ProgramVersionID = &otherVersion
	if _, err := store.InsertForAssignment(ctx, other); !errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
		t.Fatalf("other version: err = %v, want ErrAssignmentChanged", err)
	}
	foreign := completion(v, v.Assignment.CompletionCycleID)
	foreign.ClientUserID = f.clientY
	if _, err := store.InsertForAssignment(ctx, foreign); !errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
		t.Fatalf("another client's row: err = %v, want ErrAssignmentChanged", err)
	}

	// The cycle of the first assignment is stale after a reassignment.
	second := f.publishSecondProgram(t, "ca-cond")
	if _, err := program.NewStore(f.env.pool).SetClientProgramAssignment(ctx, f.trainerUserID, f.trainerID, f.clientX, &second); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if _, err := store.InsertForAssignment(ctx, completion(v, v.Assignment.CompletionCycleID)); !errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
		t.Fatalf("stale cycle after a reassignment: err = %v, want ErrAssignmentChanged", err)
	}
	if n := f.completionRows(t, f.clientX); n != 0 {
		t.Fatalf("rows = %d, want none", n)
	}

	// With the current ids the insert writes, takes the program from the assignment row,
	// and the unique key still answers a repeat.
	now := f.view(t, f.clientX)
	good := completion(now, now.Assignment.CompletionCycleID)
	got, err := store.InsertForAssignment(ctx, good)
	if err != nil {
		t.Fatalf("current ids: %v", err)
	}
	if got.ProgramID == nil || *got.ProgramID != second || got.Source != "app" {
		t.Fatalf("row = %+v, want the program of the assignment row and source app", got)
	}
	if _, err := store.InsertForAssignment(ctx, good); !errors.Is(err, workoutcompletion.ErrAlreadyCompleted) {
		t.Fatalf("repeat: err = %v, want ErrAlreadyCompleted", err)
	}
}

// The snapshot is the day as this client sees it: a block that is hidden from them is
// not in it, although it is in the version.
func TestClientApp_markDone_snapshotLeavesOutHiddenBlocks(t *testing.T) {
	f := newAppFixture(t, "ca-snap")
	ctx := context.Background()
	if _, err := f.env.pool.Exec(ctx, `
UPDATE mentorix.program_version_week_day_blocks SET instruction = 'HIDDEN-BLOCK' WHERE block_key = $1`, f.blocks[1].BlockKey); err != nil {
		t.Fatalf("mark the hidden block: %v", err)
	}
	snapshot := func(client uuid.UUID) (string, int) {
		t.Helper()
		v := f.view(t, client)
		got, err := f.env.svc.CompleteClientDay(ctx, client, f.trainerID, requestFor(v, firstDayKey(t, v), "ok"))
		if err != nil {
			t.Fatalf("mark: %v", err)
		}
		var text string
		var blocks int
		if err := f.env.pool.QueryRow(ctx, `SELECT day_snapshot::text, jsonb_array_length(day_snapshot->'blocks') FROM mentorix.client_workout_completions WHERE id = $1`, got.ID).Scan(&text, &blocks); err != nil {
			t.Fatalf("read snapshot: %v", err)
		}
		return text, blocks
	}

	text, blocks := snapshot(f.clientX)
	if blocks != 2 || strings.Contains(text, "HIDDEN-BLOCK") {
		t.Fatalf("clientX snapshot has %d blocks: %s; want 2 and no hidden block", blocks, text)
	}
	text, blocks = snapshot(f.clientY)
	if blocks != 3 || !strings.Contains(text, "HIDDEN-BLOCK") {
		t.Fatalf("clientY snapshot has %d blocks: %s; want all 3", blocks, text)
	}
}

func TestClientApp_markDone_aDayWithOnlyHiddenBlocksIsNotFound(t *testing.T) {
	f := newAppFixture(t, "ca-allhidden")
	ctx := context.Background()
	x := f.view(t, f.clientX)
	y := f.view(t, f.clientY)
	dayKey := y.Program.Weeks[0].Days[1].DayKey
	if got := x.Program.Weeks[0].Days[1]; got.DayKey != dayKey || len(got.Blocks) != 0 {
		t.Fatalf("clientX day 2 = %+v, want the same day with no visible block", got)
	}

	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientX, f.trainerID, requestFor(x, dayKey, "ok")); !errors.Is(err, trainerclient.ErrDayNotFound) {
		t.Fatalf("clientX: err = %v, want ErrDayNotFound", err)
	}
	if n := f.completionRows(t, f.clientX); n != 0 {
		t.Fatalf("rows = %d, want none", n)
	}
	if _, err := f.env.svc.CompleteClientDay(ctx, f.clientY, f.trainerID, requestFor(y, dayKey, "ok")); err != nil {
		t.Fatalf("clientY may mark the day they see: %v", err)
	}
}

// The program answer is built for the app. Fields that belong to the trainer's editor
// (owner ids, counters, draft state) are not in it.
func TestClientApp_programView_hasNoTrainerOnlyFields(t *testing.T) {
	f := newAppFixture(t, "ca-shape")

	raw, err := json.Marshal(f.view(t, f.clientX))
	if err != nil {
		t.Fatal(err)
	}

	var body struct {
		Program map[string]json.RawMessage `json:"program"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"id", "created_by", "created_by_name", "modified_by", "modified_at", "created_at", "status", "assignment_count",
		"training_days_count", "has_unpublished_changes", "latest_program_version_id", "latest_client_plan_at", "deleted_at",
	} {
		if _, present := body.Program[key]; present {
			t.Errorf("program has the trainer-only key %q: %s", key, raw)
		}
	}
	for _, key := range []string{"program_id", "name", "name_ru", "description", "description_ru", "category", "difficulty", "preview_image_url", "weeks"} {
		if _, present := body.Program[key]; !present {
			t.Errorf("program lacks %q: %s", key, raw)
		}
	}
	if len(body.Program) != 9 {
		t.Errorf("program has %d keys, want exactly 9: %s", len(body.Program), raw)
	}
}

// A token outlives the account it was issued for. The user is gone, so there is no link
// and the answer is the plain 403, not a 500 from a missing row.
func TestClientApp_deletedUsersTokenIsRefusedNotAnError(t *testing.T) {
	m := newMountedEnv(t)
	ctx := context.Background()
	client := newClientWithIdentity(t, m.store, auth.ProviderGoogle, "g-ca-deleted", "ca-deleted@test.com")
	linkToNewTrainer(t, m.pool, m.store, client, "ca-deleted-trainer@test.com")
	var trainerID uuid.UUID
	if err := m.pool.QueryRow(ctx, `SELECT trainer_id FROM mentorix.trainer_clients WHERE client_user_id = $1`, client).Scan(&trainerID); err != nil {
		t.Fatalf("trainer id: %v", err)
	}
	token := m.accessToken(t, client)
	if rec := m.do(http.MethodDelete, "/auth/me", token, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete account: %d %s", rec.Code, rec.Body)
	}
	base := "/client/trainers/" + trainerID.String()
	body := `{"program_assignment_id":"` + uuid.NewString() + `","program_version_id":"` + uuid.NewString() +
		`","completion_cycle_id":"` + uuid.NewString() + `","day_key":"` + uuid.NewString() + `","result_text":"ok"}`

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"program":     m.do(http.MethodGet, base+"/program", token, ""),
		"completions": m.do(http.MethodPost, base+"/completions", token, body),
	} {
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"client_not_linked"`) {
			t.Errorf("%s: %d %s, want 403 client_not_linked", name, rec.Code, rec.Body)
		}
	}
	if rec := m.do(http.MethodGet, base+"/analytics", token, ""); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code":"client_not_found"`) {
		t.Errorf("analytics: %d %s, want 404 client_not_found", rec.Code, rec.Body)
	}
}

// currentCompletion is the completion the app would write for clientX's current assignment.
func (f appFixture) currentCompletion(t *testing.T) workoutcompletion.Completion {
	t.Helper()
	v := f.view(t, f.clientX)
	assignment, version := v.Assignment.AssignmentID, v.Assignment.ProgramVersionID
	return workoutcompletion.Completion{
		ClientUserID: f.clientX, TrainerID: f.trainerID, CompletedAt: time.Now().UTC(),
		ProgramAssignmentID: &assignment, ProgramVersionID: &version, CompletionCycleID: v.Assignment.CompletionCycleID,
		DayKey: firstDayKey(t, v), WeekNumber: 1, DayNumber: 1, ProgramName: "p", ProgramNameRu: "p",
		DaySnapshot: []byte(`{"blocks":[]}`), ResultText: "ok", Source: workoutcompletion.SourceApp,
	}
}

func isDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40P01"
}

// insertInBackground runs the conditional insert in a goroutine under a 5 second context
// and reports its error once on the returned buffered channel.
func insertInBackground(f appFixture, c workoutcompletion.Completion) <-chan error {
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := workoutcompletion.NewStore(f.env.pool).InsertForAssignment(ctx, c)
		done <- err
	}()
	return done
}

// waitForInsert reads the insert's result once. Not being done yet is allowed only before
// the lock holder commits, so the caller commits first.
func waitForInsert(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(6 * time.Second):
		t.Fatal("the insert did not finish within 6 seconds after the lock holder committed")
		return nil
	}
}

func insertIsWaiting(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("the insert finished (%v) while the other transaction held its locks; the scenario does not hold", err)
	case <-time.After(400 * time.Millisecond):
	}
}

// The trainer's reassignment locks the trainer row and then updates the assignment. The
// insert must not take a lock on the assignment row that makes the two wait for each
// other: Postgres would abort one of them with a deadlock.
func TestClientApp_conditionalInsert_doesNotDeadlockWithAReassignment(t *testing.T) {
	f := newAppFixture(t, "ca-dl-reassign")
	ctx := context.Background()
	c := f.currentCompletion(t)
	tx, err := f.env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM mentorix.trainers WHERE id = $1 FOR UPDATE`, f.trainerID); err != nil {
		t.Fatalf("lock the trainer: %v", err)
	}

	done := insertInBackground(f, c)
	insertIsWaiting(t, done)
	if _, err := tx.Exec(ctx, `
UPDATE mentorix.program_assignments SET completion_cycle_id = gen_random_uuid(), assigned_at = now()
WHERE client_user_id = $1 AND trainer_id = $2`, f.clientX, f.trainerID); err != nil {
		if isDeadlock(err) {
			t.Fatalf("the reassignment was aborted as a deadlock victim: %v", err)
		}
		t.Fatalf("reassign: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit the reassignment: %v", err)
	}

	err = waitForInsert(t, done)
	if isDeadlock(err) {
		t.Fatalf("the insert was aborted as a deadlock victim: %v", err)
	}
	// The insert read the assignment before the commit, so it may land in the old cycle;
	// it must never fail any other way.
	if err != nil && !errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
		t.Fatalf("err = %v, want a row or ErrAssignmentChanged", err)
	}
}

// Account deletion locks the user row and then deletes the client's assignments.
func TestClientApp_conditionalInsert_doesNotDeadlockWithAccountDeletion(t *testing.T) {
	f := newAppFixture(t, "ca-dl-delete")
	ctx := context.Background()
	c := f.currentCompletion(t)
	tx, err := f.env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM mentorix.users WHERE id = $1 FOR UPDATE`, f.clientX); err != nil {
		t.Fatalf("lock the user: %v", err)
	}

	done := insertInBackground(f, c)
	insertIsWaiting(t, done)
	if _, err := tx.Exec(ctx, `DELETE FROM mentorix.program_assignments WHERE client_user_id = $1`, f.clientX); err != nil {
		if isDeadlock(err) {
			t.Fatalf("the deletion was aborted as a deadlock victim: %v", err)
		}
		t.Fatalf("delete the assignments: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit the deletion: %v", err)
	}

	err = waitForInsert(t, done)
	if isDeadlock(err) {
		t.Fatalf("the insert was aborted as a deadlock victim: %v", err)
	}
	if err != nil && !errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
		t.Fatalf("err = %v, want a row or ErrAssignmentChanged", err)
	}
}

// The version the insert read can be gone by the time its foreign key is checked: a
// reassignment moves the assignment to another version and the old one is deleted. The
// foreign key violation (23503) is then the same answer as a changed assignment.
func TestClientApp_conditionalInsert_aVersionDeletedMidInsertIsAChangedAssignment(t *testing.T) {
	f := newAppFixture(t, "ca-fk")
	ctx := context.Background()
	c := f.currentCompletion(t)
	second := f.publishSecondProgram(t, "ca-fk")
	var newVersion uuid.UUID
	if err := f.env.pool.QueryRow(ctx, `SELECT id FROM mentorix.program_versions WHERE program_id = $1 ORDER BY version_number DESC LIMIT 1`, second).Scan(&newVersion); err != nil {
		t.Fatalf("version of the second program: %v", err)
	}
	tx, err := f.env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
UPDATE mentorix.program_assignments SET program_id = $3, program_version_id = $4, completion_cycle_id = gen_random_uuid()
WHERE client_user_id = $1 AND trainer_id = $2`, f.clientX, f.trainerID, second, newVersion); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	// Clients of the old program still point at it, so remove the version only for this test's row set.
	if _, err := tx.Exec(ctx, `UPDATE mentorix.program_assignments SET program_version_id = $2 WHERE program_version_id = $1`, *c.ProgramVersionID, newVersion); err != nil {
		t.Fatalf("move the other assignments: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM mentorix.program_versions WHERE id = $1`, *c.ProgramVersionID); err != nil {
		t.Fatalf("delete the old version: %v", err)
	}

	done := insertInBackground(f, c)
	insertIsWaiting(t, done)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := waitForInsert(t, done); !errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
		t.Fatalf("err = %v, want ErrAssignmentChanged", err)
	}
	if n := f.completionRows(t, f.clientX); n != 0 {
		t.Fatalf("rows = %d, want none", n)
	}
}

// A block that committed before the insert is refused at the insert itself, not only by
// the service's earlier check.
func TestClientApp_conditionalInsert_refusesABlockedLink(t *testing.T) {
	f := newAppFixture(t, "ca-blockpred")
	ctx := context.Background()
	c := f.currentCompletion(t)
	if _, err := f.env.pool.Exec(ctx, `UPDATE mentorix.trainer_clients SET status = 'blocked' WHERE client_user_id = $1 AND trainer_id = $2`, f.clientX, f.trainerID); err != nil {
		t.Fatal(err)
	}

	_, err := workoutcompletion.NewStore(f.env.pool).InsertForAssignment(ctx, c)

	if !errors.Is(err, workoutcompletion.ErrAssignmentChanged) {
		t.Fatalf("err = %v, want ErrAssignmentChanged", err)
	}
	if n := f.completionRows(t, f.clientX); n != 0 {
		t.Fatalf("rows = %d, want none", n)
	}
}
