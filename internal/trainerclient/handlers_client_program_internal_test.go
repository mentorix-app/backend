package trainerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/workoutcompletion"
)

type fakeClientApp struct {
	program     ClientProgramResponse
	programErr  error
	completion  ClientCompletionResponse
	completeErr error

	programCalls  int
	completeCalls int
	gotUser       uuid.UUID
	gotTrainer    uuid.UUID
	gotRequest    ClientCompletionRequest
}

func (f *fakeClientApp) GetClientProgram(_ context.Context, clientUserID, trainerID uuid.UUID) (ClientProgramResponse, error) {
	f.programCalls++
	f.gotUser, f.gotTrainer = clientUserID, trainerID
	return f.program, f.programErr
}

func (f *fakeClientApp) CompleteClientDay(_ context.Context, clientUserID, trainerID uuid.UUID, req ClientCompletionRequest) (ClientCompletionResponse, error) {
	f.completeCalls++
	f.gotUser, f.gotTrainer, f.gotRequest = clientUserID, trainerID, req
	return f.completion, f.completeErr
}

func clientAppEcho(f *fakeClientApp) *echo.Echo {
	h := NewHandlers(nil, nil, clientInviteTestSecret)
	h.clientApp = f
	e := echo.New()
	e.HTTPErrorHandler = httpx.ErrorHandler
	h.Mount(e)
	return e
}

func doClientApp(e *echo.Echo, method, target, authz, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if authz != "" {
		req.Header.Set(echo.HeaderAuthorization, authz)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func completionBody(assignmentID, versionID, dayKey uuid.UUID, text string) string {
	return fmt.Sprintf(`{"program_assignment_id":%q,"program_version_id":%q,"completion_cycle_id":%q,"day_key":%q,"result_text":%q}`,
		assignmentID, versionID, cycleOf(assignmentID), dayKey, text)
}

// cycleOf gives a fixed cycle id per assignment id, so a test can predict the request.
func cycleOf(assignmentID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(assignmentID.String()))
}

func TestClientProgram_success(t *testing.T) {
	userID, trainerID := uuid.New(), uuid.New()
	assignmentID, programID, versionID, cycleID, dayKey := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	assignedAt := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	f := &fakeClientApp{program: ClientProgramResponse{
		TrainerID:          trainerID,
		TrainerDisplayName: "Anna",
		Assignment: &ClientProgramAssignment{
			AssignmentID: assignmentID, ProgramID: programID, ProgramVersionID: versionID, CompletionCycleID: cycleID, AssignedAt: assignedAt,
		},
		Program:       &ClientProgram{ProgramID: programID, Weeks: []program.Week{}},
		CompletedDays: []ClientCompletedDay{{DayKey: dayKey}},
	}}

	rec := doClientApp(clientAppEcho(f), http.MethodGet, "/client/trainers/"+trainerID.String()+"/program", bearerFor(t, userID), "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.programCalls != 1 || f.gotUser != userID || f.gotTrainer != trainerID {
		t.Fatalf("service called %d times for user %v trainer %v, want once for %v and %v", f.programCalls, f.gotUser, f.gotTrainer, userID, trainerID)
	}
	got := decodeJSONObject(t, rec)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	if !sameSet(keys, []string{"trainer_id", "trainer_display_name", "assignment", "program", "completed_days"}) {
		t.Fatalf("body keys = %v", keys)
	}
	if got["trainer_id"] != trainerID.String() || got["trainer_display_name"] != "Anna" {
		t.Fatalf("trainer fields = %v / %v", got["trainer_id"], got["trainer_display_name"])
	}
	wantAssignment := map[string]any{
		"assignment_id": assignmentID.String(), "program_id": programID.String(),
		"program_version_id": versionID.String(), "completion_cycle_id": cycleID.String(), "assigned_at": "2026-10-01T08:00:00Z",
	}
	if !reflect.DeepEqual(got["assignment"], wantAssignment) {
		t.Fatalf("assignment = %v, want exactly %v", got["assignment"], wantAssignment)
	}
	if !reflect.DeepEqual(got["completed_days"], []any{map[string]any{"day_key": dayKey.String()}}) {
		t.Fatalf("completed_days = %v", got["completed_days"])
	}
}

func TestClientProgram_noAssignmentIsNullsAndAnEmptyArray(t *testing.T) {
	trainerID := uuid.New()
	// A nil slice would be written as null; the contract promises an array.
	f := &fakeClientApp{program: ClientProgramResponse{TrainerID: trainerID, TrainerDisplayName: "Anna"}}

	rec := doClientApp(clientAppEcho(f), http.MethodGet, "/client/trainers/"+trainerID.String()+"/program", bearerFor(t, uuid.New()), "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := fmt.Sprintf(`{"trainer_id":%q,"trainer_display_name":"Anna","assignment":null,"program":null,"completed_days":[]}`, trainerID)
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("body = %s, want exactly %s", got, want)
	}
}

func TestClientProgram_requiresToken(t *testing.T) {
	f := &fakeClientApp{}
	e := clientAppEcho(f)
	target := "/client/trainers/" + uuid.NewString() + "/program"
	for name, authz := range map[string]string{"missing": "", "malformed": "Bearer nope"} {
		rec := doClientApp(e, http.MethodGet, target, authz, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
		}
	}
	if f.programCalls != 0 {
		t.Fatalf("service called %d times without a valid token", f.programCalls)
	}
}

func TestClientProgram_badTrainerID(t *testing.T) {
	f := &fakeClientApp{}

	rec := doClientApp(clientAppEcho(f), http.MethodGet, "/client/trainers/not-a-uuid/program", bearerFor(t, uuid.New()), "")

	assertCodedError(t, rec, http.StatusBadRequest, "bad_request", "invalid id")
	if f.programCalls != 0 {
		t.Fatalf("service called %d times for a bad id", f.programCalls)
	}
}

func TestClientProgram_onlyThePathNamesTheTrainer(t *testing.T) {
	userID, pathTrainer, other := uuid.New(), uuid.New(), uuid.New()
	f := &fakeClientApp{}
	target := "/client/trainers/" + pathTrainer.String() + "/program?trainer_id=" + other.String() + "&client_user_id=" + other.String()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set(echo.HeaderAuthorization, bearerFor(t, userID))
	req.Header.Set("X-Trainer-ID", other.String())
	rec := httptest.NewRecorder()

	clientAppEcho(f).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.gotTrainer != pathTrainer || f.gotUser != userID {
		t.Fatalf("service got trainer %v user %v, want path trainer %v and token user %v", f.gotTrainer, f.gotUser, pathTrainer, userID)
	}
}

func TestClientProgram_errorCodes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"not linked", program.ErrClientNotLinked, 403, "client_not_linked", "client is not linked to trainer"},
		{"blocked", program.ErrClientBlocked, 422, "client_blocked", "client is blocked"},
		{"wrapped blocked", fmt.Errorf("view: %w", program.ErrClientBlocked), 422, "client_blocked", "view: client is blocked"},
		{"unexpected hides the cause", errors.New("dial tcp 10.0.0.5:5432: refused"), 500, "internal_error", "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeClientApp{programErr: tt.err}
			rec := doClientApp(clientAppEcho(f), http.MethodGet, "/client/trainers/"+uuid.NewString()+"/program", bearerFor(t, uuid.New()), "")
			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

func TestClientCompletion_success(t *testing.T) {
	userID, trainerID := uuid.New(), uuid.New()
	assignmentID, versionID, dayKey, completionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	completedAt := time.Date(2026, 10, 18, 7, 30, 0, 0, time.UTC)
	f := &fakeClientApp{completion: ClientCompletionResponse{
		ID: completionID, DayKey: dayKey, WeekNumber: 2, DayNumber: 3, CompletedAt: completedAt, ResultText: "felt strong",
	}}

	rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+trainerID.String()+"/completions",
		bearerFor(t, userID), completionBody(assignmentID, versionID, dayKey, "felt strong"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := map[string]any{
		"id": completionID.String(), "day_key": dayKey.String(), "week_number": 2.0, "day_number": 3.0,
		"completed_at": "2026-10-18T07:30:00Z", "result_text": "felt strong",
	}
	if got := decodeJSONObject(t, rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want exactly %v", got, want)
	}
	wantReq := ClientCompletionRequest{ProgramAssignmentID: assignmentID, ProgramVersionID: versionID, CompletionCycleID: cycleOf(assignmentID), DayKey: dayKey, ResultText: "felt strong"}
	if f.completeCalls != 1 || f.gotUser != userID || f.gotTrainer != trainerID || f.gotRequest != wantReq {
		t.Fatalf("service got user %v trainer %v request %+v (%d calls)", f.gotUser, f.gotTrainer, f.gotRequest, f.completeCalls)
	}
}

func TestClientCompletion_textLengthIsCountedInCharacters(t *testing.T) {
	ids := [3]uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	tests := []struct {
		name       string
		text       string
		wantStatus int
	}{
		{"exactly the limit in two-byte characters", strings.Repeat("я", workoutcompletion.MaxResultTextLen), http.StatusCreated},
		{"one over the limit", strings.Repeat("я", workoutcompletion.MaxResultTextLen+1), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeClientApp{}
			rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions",
				bearerFor(t, uuid.New()), completionBody(ids[0], ids[1], ids[2], tt.text))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus == http.StatusBadRequest {
				assertCodedError(t, rec, 400, "bad_request", "result_text too long")
				if f.completeCalls != 0 {
					t.Fatal("service called for a too long text")
				}
			}
		})
	}
}

func TestClientCompletion_badRequests(t *testing.T) {
	a, v, c, d := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	nilID := uuid.Nil.String()
	body := func(assignment, version, cycle, day, text string) string {
		return fmt.Sprintf(`{"program_assignment_id":%q,"program_version_id":%q,"completion_cycle_id":%q,"day_key":%q,"result_text":%q}`, assignment, version, cycle, day, text)
	}
	tests := []struct {
		name        string
		trainerPath string
		body        string
		wantMessage string
	}{
		{"not json", uuid.NewString(), `{`, "invalid json"},
		{"empty body", uuid.NewString(), ``, "program_assignment_id required"},
		{"id of the wrong type", uuid.NewString(), `{"program_assignment_id":"x","program_version_id":"` + v + `","completion_cycle_id":"` + c + `","day_key":"` + d + `","result_text":"ok"}`, "invalid json"},
		{"assignment id missing", uuid.NewString(), `{"program_version_id":"` + v + `","completion_cycle_id":"` + c + `","day_key":"` + d + `","result_text":"ok"}`, "program_assignment_id required"},
		{"assignment id zero", uuid.NewString(), body(nilID, v, c, d, "ok"), "program_assignment_id required"},
		{"version id missing", uuid.NewString(), `{"program_assignment_id":"` + a + `","completion_cycle_id":"` + c + `","day_key":"` + d + `","result_text":"ok"}`, "program_version_id required"},
		{"version id zero", uuid.NewString(), body(a, nilID, c, d, "ok"), "program_version_id required"},
		{"cycle id missing", uuid.NewString(), `{"program_assignment_id":"` + a + `","program_version_id":"` + v + `","day_key":"` + d + `","result_text":"ok"}`, "completion_cycle_id required"},
		{"cycle id zero", uuid.NewString(), body(a, v, nilID, d, "ok"), "completion_cycle_id required"},
		{"day key missing", uuid.NewString(), `{"program_assignment_id":"` + a + `","program_version_id":"` + v + `","completion_cycle_id":"` + c + `","result_text":"ok"}`, "day_key required"},
		{"day key zero", uuid.NewString(), body(a, v, c, nilID, "ok"), "day_key required"},
		{"text missing", uuid.NewString(), `{"program_assignment_id":"` + a + `","program_version_id":"` + v + `","completion_cycle_id":"` + c + `","day_key":"` + d + `"}`, "result_text required"},
		{"text empty", uuid.NewString(), body(a, v, c, d, ""), "result_text required"},
		{"text blank", uuid.NewString(), body(a, v, c, d, " \t\n "), "result_text required"},
		{"trainer id bad", "not-a-uuid", body(a, v, c, d, "ok"), "invalid id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeClientApp{}
			rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+tt.trainerPath+"/completions", bearerFor(t, uuid.New()), tt.body)
			assertCodedError(t, rec, http.StatusBadRequest, "bad_request", tt.wantMessage)
			if f.completeCalls != 0 {
				t.Fatalf("service called %d times for a bad request", f.completeCalls)
			}
		})
	}
}

// A NUL byte makes Postgres refuse the text and would answer 500; the other C0
// control characters have no place in a workout note either. Line breaks and tabs do.
func TestClientCompletion_controlCharactersInTheText(t *testing.T) {
	a, v, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	bodyWith := func(text string) string {
		return fmt.Sprintf(`{"program_assignment_id":%q,"program_version_id":%q,"completion_cycle_id":%q,"day_key":%q,"result_text":%s}`,
			a, v, c, d, mustJSONString(t, text))
	}
	refused := map[string]string{
		"NUL":                 "before\x00after",
		"NUL alone":           "\x00",
		"start of heading":    "a\x01b",
		"bell":                "a\x07b",
		"backspace":           "a\x08b",
		"vertical tab":        "a\x0bb",
		"form feed":           "a\x0cb",
		"escape":              "a\x1bb",
		"unit separator":      "a\x1fb",
		"NUL at the very end": "text\x00",
	}
	for name, text := range refused {
		t.Run("refuses "+name, func(t *testing.T) {
			f := &fakeClientApp{}
			rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions", bearerFor(t, uuid.New()), bodyWith(text))
			assertCodedError(t, rec, http.StatusBadRequest, "bad_request", "result_text contains control characters")
			if f.completeCalls != 0 {
				t.Fatal("service called for a text with a control character")
			}
		})
	}
	t.Run("the escape sequence in raw JSON", func(t *testing.T) {
		f := &fakeClientApp{}
		raw := fmt.Sprintf(`{"program_assignment_id":%q,"program_version_id":%q,"completion_cycle_id":%q,"day_key":%q,"result_text":"x\u0000y"}`, a, v, c, d)
		rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions", bearerFor(t, uuid.New()), raw)
		assertCodedError(t, rec, http.StatusBadRequest, "bad_request", "result_text contains control characters")
	})
	for name, text := range map[string]string{
		"newline":            "line one\nline two",
		"carriage return":    "line one\r\nline two",
		"tab":                "sets:\t3x10",
		"emoji and Cyrillic": "Присед 🔥",
		"DEL":                "a\x7fb",
	} {
		t.Run("accepts "+name, func(t *testing.T) {
			f := &fakeClientApp{}
			rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions", bearerFor(t, uuid.New()), bodyWith(text))
			if rec.Code != http.StatusCreated || f.completeCalls != 1 {
				t.Fatalf("status = %d with %d service calls, want 201 and one; body %s", rec.Code, f.completeCalls, rec.Body)
			}
		})
	}
}

func mustJSONString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// POST completions takes a small JSON object; the route refuses a body above 16000
// bytes (Echo reads "16K" as decimal kilobytes), with or without a Content-Length.
func TestClientCompletion_bodyLimit(t *testing.T) {
	const limit = 16000
	base := completionBody(uuid.New(), uuid.New(), uuid.New(), "ok")
	padded := func(n int) string { return base + strings.Repeat(" ", n-len(base)) }

	t.Run("a body of exactly the limit is accepted", func(t *testing.T) {
		f := &fakeClientApp{}
		rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions", bearerFor(t, uuid.New()), padded(limit))
		if rec.Code != http.StatusCreated || f.completeCalls != 1 {
			t.Fatalf("status = %d with %d calls, body %s", rec.Code, f.completeCalls, rec.Body)
		}
	})
	t.Run("one byte over is refused", func(t *testing.T) {
		f := &fakeClientApp{}
		rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions", bearerFor(t, uuid.New()), padded(limit+1))
		assertCodedError(t, rec, http.StatusRequestEntityTooLarge, "payload_too_large", "Request Entity Too Large")
		if f.completeCalls != 0 {
			t.Fatal("service called for an oversized body")
		}
	})
	t.Run("an oversized body without a length is refused too", func(t *testing.T) {
		f := &fakeClientApp{}
		// The JSON decoder reads in growing chunks and ends at the object's end, so the
		// body has to be well over the limit for the reader to trip before the object is whole.
		big := completionBody(uuid.New(), uuid.New(), uuid.New(), strings.Repeat("a", 10*limit))
		req := httptest.NewRequest(http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions", io.NopCloser(strings.NewReader(big)))
		req.ContentLength = -1
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderAuthorization, bearerFor(t, uuid.New()))
		rec := httptest.NewRecorder()
		clientAppEcho(f).ServeHTTP(rec, req)
		assertCodedError(t, rec, http.StatusRequestEntityTooLarge, "payload_too_large", "Request Entity Too Large")
	})
	t.Run("a bad token is answered before the body is read", func(t *testing.T) {
		rec := doClientApp(clientAppEcho(&fakeClientApp{}), http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions", "", padded(limit+1))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}

func TestClientCompletion_requiresToken(t *testing.T) {
	f := &fakeClientApp{}
	e := clientAppEcho(f)
	target := "/client/trainers/" + uuid.NewString() + "/completions"
	body := completionBody(uuid.New(), uuid.New(), uuid.New(), "ok")
	for name, authz := range map[string]string{"missing": "", "malformed": "Bearer nope"} {
		rec := doClientApp(e, http.MethodPost, target, authz, body)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
		}
	}
	if f.completeCalls != 0 {
		t.Fatalf("service called %d times without a valid token", f.completeCalls)
	}
}

func TestClientCompletion_onlyThePathNamesTheTrainer(t *testing.T) {
	userID, pathTrainer, other := uuid.New(), uuid.New(), uuid.New()
	f := &fakeClientApp{}
	body := fmt.Sprintf(`{"trainer_id":%q,"client_user_id":%q,"program_assignment_id":%q,"program_version_id":%q,"completion_cycle_id":%q,"day_key":%q,"result_text":"ok"}`,
		other, other, uuid.New(), uuid.New(), uuid.New(), uuid.New())

	rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+pathTrainer.String()+"/completions?trainer_id="+other.String(), bearerFor(t, userID), body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if f.gotTrainer != pathTrainer || f.gotUser != userID {
		t.Fatalf("service got trainer %v user %v, want path trainer %v and token user %v", f.gotTrainer, f.gotUser, pathTrainer, userID)
	}
}

func TestClientCompletion_errorCodes(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"not linked", program.ErrClientNotLinked, 403, "client_not_linked", "client is not linked to trainer"},
		{"blocked", program.ErrClientBlocked, 422, "client_blocked", "client is blocked"},
		{"day not found", ErrDayNotFound, 404, "day_not_found", "day not found"},
		{"program changed", ErrProgramChanged, 409, "program_changed", "program changed"},
		{"already completed", workoutcompletion.ErrAlreadyCompleted, 409, "workout_already_completed", "workout already completed"},
		{"wrapped already completed", fmt.Errorf("insert: %w", workoutcompletion.ErrAlreadyCompleted), 409, "workout_already_completed", "insert: workout already completed"},
		{"service validation", fmt.Errorf("%w: result_text too long", workoutcompletion.ErrValidation), 400, "bad_request", "validation failed: result_text too long"},
		{"unexpected hides the cause", errors.New("dial tcp 10.0.0.5:5432: refused"), 500, "internal_error", "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeClientApp{completeErr: tt.err}
			rec := doClientApp(clientAppEcho(f), http.MethodPost, "/client/trainers/"+uuid.NewString()+"/completions",
				bearerFor(t, uuid.New()), completionBody(uuid.New(), uuid.New(), uuid.New(), "ok"))
			assertCodedError(t, rec, tt.wantStatus, tt.wantCode, tt.wantMessage)
		})
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
