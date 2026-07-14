package telegrambot

import (
	"context"
	"errors"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
	"mentorix-backend/internal/workoutcompletion"
)

func TestParseProgramDayDoneCallback(t *testing.T) {
	w, d, ok := parseProgramDayDoneCallback("program_day_done:2:3")
	if !ok || w != 2 || d != 3 {
		t.Fatalf("got %d %d %v", w, d, ok)
	}
	if _, _, ok := parseProgramDayDoneCallback("program_day:1:1"); ok {
		t.Fatal("should reject non-done prefix")
	}
	if _, _, ok := parseProgramDayDoneCallback("program_day_done:x:1"); ok {
		t.Fatal("should reject bad week")
	}
}

func TestProgramDayKeyboard_completed(t *testing.T) {
	weeks := []program.Week{{
		WeekNumber: 1,
		Days: []program.Day{
			{DayNumber: 1, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{ExerciseName: "a"}}}}},
			{DayNumber: 2, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{ExerciseName: "b"}}}}},
		},
	}}
	kb := programDayKeyboard(weeks, 1, 1, true)
	if len(kb.InlineKeyboard) < 1 || kb.InlineKeyboard[0][0].Text != btnCompleteWorkoutDone {
		t.Fatalf("%+v", kb)
	}
	kb = programDayKeyboard(weeks, 1, 1, false)
	if kb.InlineKeyboard[0][0].Text != btnCompleteWorkout {
		t.Fatalf("%+v", kb)
	}
}

type fakeWorkoutStore struct {
	exists     bool
	complete   error
	insertions int
}

func (f *fakeWorkoutStore) Exists(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.exists, nil
}
func (f *fakeWorkoutStore) ListCompletedDayKeys(context.Context, uuid.UUID) (map[uuid.UUID]struct{}, error) {
	return nil, nil
}
func (f *fakeWorkoutStore) Insert(_ context.Context, c workoutcompletion.Completion) (workoutcompletion.Completion, error) {
	if f.complete != nil {
		return workoutcompletion.Completion{}, f.complete
	}
	f.insertions++
	c.ID = uuid.New()
	return c, nil
}

func programWithAssignment(dayKey uuid.UUID) trainerclient.TelegramProgramResponse {
	cycle := uuid.New()
	sets, reps := "3", "8"
	ex := program.DayExercise{ExerciseNameRu: "Присед", Sets: &sets, Reps: &reps}
	return trainerclient.TelegramProgramResponse{
		TrainerID:          uuid.New(),
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		Assignment: &trainerclient.ClientProgramSummary{
			AssignmentID:      uuid.New(),
			ProgramID:         uuid.New(),
			ProgramVersionID:  uuid.New(),
			CompletionCycleID: cycle,
		},
		Program: &program.Detail{
			Program: program.Program{Name: "Force", NameRu: "Сила"},
			Weeks: []program.Week{{
				WeekNumber: 1,
				Days: []program.Day{
					{DayNumber: 1, DayKey: dayKey, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
					{DayNumber: 2, DayKey: uuid.New(), Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
				},
			}},
		},
	}
}

func TestBot_workoutCompleteFlow(t *testing.T) {
	dayKey := uuid.New()
	api := &fakeTelegramAPI{}
	clients := &fakeTrainerClient{program: programWithAssignment(dayKey)}
	store := &fakeWorkoutStore{}
	pending := workoutcompletion.NewMemoryPendingStore()
	bot := New(api, clients, WithWorkoutCompletions(workoutcompletion.New(store), pending))

	chat := &tgbotapi.Chat{ID: 7}
	user := &tgbotapi.User{ID: 42}

	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "cb-done",
		Data:    programDayDoneCallbackData(1, 1),
		From:    user,
		Message: &tgbotapi.Message{Chat: chat},
	})
	if len(api.sent) == 0 || !strings.Contains(api.sent[len(api.sent)-1].Text, "результат") {
		t.Fatalf("want ask result, sent=%+v", api.sent)
	}

	api.sent = nil
	bot.handleMessage(context.Background(), &tgbotapi.Message{
		Chat: chat,
		From: user,
		Text: "  heavy squats  ",
	})
	if store.insertions != 1 {
		t.Fatalf("insertions = %d", store.insertions)
	}
	if len(api.sent) == 0 {
		t.Fatal("expected day re-render after complete")
	}
}

func TestBot_workoutAlreadyCompletedToast(t *testing.T) {
	dayKey := uuid.New()
	api := &fakeTelegramAPI{}
	clients := &fakeTrainerClient{program: programWithAssignment(dayKey)}
	store := &fakeWorkoutStore{exists: true}
	pending := workoutcompletion.NewMemoryPendingStore()
	bot := New(api, clients, WithWorkoutCompletions(workoutcompletion.New(store), pending))

	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "cb-done",
		Data:    programDayDoneCallbackData(1, 1),
		From:    &tgbotapi.User{ID: 42},
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if len(api.sent) != 0 {
		t.Fatalf("already completed should only toast, got sent=%+v", api.sent)
	}
}

func TestBot_pendingCancel(t *testing.T) {
	dayKey := uuid.New()
	api := &fakeTelegramAPI{}
	clients := &fakeTrainerClient{program: programWithAssignment(dayKey)}
	pending := workoutcompletion.NewMemoryPendingStore()
	bot := New(api, clients, WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), pending))

	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{
		WeekNumber: 1,
		DayNumber:  1,
		DayKey:     dayKey,
	})
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "cancel",
		Data:    programDayDoneCancelCallbackData,
		From:    &tgbotapi.User{ID: 42},
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if _, ok, _ := pending.Get(context.Background(), "42"); ok {
		t.Fatal("pending should be cleared")
	}
	if len(api.sent) == 0 {
		t.Fatal("expected day screen after cancel")
	}
}

func TestBot_menuClearsPending(t *testing.T) {
	pending := workoutcompletion.NewMemoryPendingStore()
	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{WeekNumber: 1, DayNumber: 1})
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{}, WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), pending))
	bot.handleMessage(context.Background(), &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 42},
		Text: btnHelp,
	})
	if _, ok, _ := pending.Get(context.Background(), "42"); ok {
		t.Fatal("menu should clear pending")
	}
}

func TestBot_emptyResultKeepsPending(t *testing.T) {
	dayKey := uuid.New()
	pending := workoutcompletion.NewMemoryPendingStore()
	prog := programWithAssignment(dayKey)
	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{
		ClientUserID:      uuid.New(),
		TrainerID:         prog.TrainerID,
		ProgramID:         prog.Assignment.ProgramID,
		ProgramVersionID:  prog.Assignment.ProgramVersionID,
		CompletionCycleID: prog.Assignment.CompletionCycleID,
		DayKey:            dayKey,
		WeekNumber:        1,
		DayNumber:         1,
	})
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{program: prog}, WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), pending))
	bot.handleMessage(context.Background(), &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 42},
		Text: "   ",
	})
	if _, ok, _ := pending.Get(context.Background(), "42"); !ok {
		t.Fatal("empty result should keep pending")
	}
	if len(api.sent) == 0 || !strings.Contains(api.sent[0].Text, "пустым") {
		t.Fatalf("sent=%+v", api.sent)
	}
}

func TestBot_resultTooLongKeepsPending(t *testing.T) {
	dayKey := uuid.New()
	pending := workoutcompletion.NewMemoryPendingStore()
	prog := programWithAssignment(dayKey)
	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{
		ClientUserID:      uuid.New(),
		TrainerID:         prog.TrainerID,
		ProgramID:         prog.Assignment.ProgramID,
		ProgramVersionID:  prog.Assignment.ProgramVersionID,
		CompletionCycleID: prog.Assignment.CompletionCycleID,
		DayKey:            dayKey,
		WeekNumber:        1,
		DayNumber:         1,
		ProgramName:       "Force",
	})
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{program: prog}, WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), pending))
	bot.handleMessage(context.Background(), &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 42},
		Text: strings.Repeat("x", workoutcompletion.MaxResultTextLen+1),
	})
	if _, ok, _ := pending.Get(context.Background(), "42"); !ok {
		t.Fatal("too long should keep pending")
	}
	if len(api.sent) == 0 || !strings.Contains(api.sent[0].Text, "1000") {
		t.Fatalf("sent=%+v", api.sent)
	}
}

func TestBot_dayDoneWithoutWorkoutsNoops(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "x",
		Data:    programDayDoneCallbackData(1, 1),
		From:    &tgbotapi.User{ID: 1},
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if len(api.sent) != 0 {
		t.Fatalf("sent=%+v", api.sent)
	}
}

func TestBot_navCallbackClearsPending(t *testing.T) {
	pending := workoutcompletion.NewMemoryPendingStore()
	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{WeekNumber: 1, DayNumber: 1})
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{}, WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), pending))
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "week",
		Data:    programWeekCallbackData(1),
		From:    &tgbotapi.User{ID: 42},
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if _, ok, _ := pending.Get(context.Background(), "42"); ok {
		t.Fatal("nav callback should clear pending")
	}
}

func TestClearWorkoutPending_nilSafe(t *testing.T) {
	bot := New(&fakeTelegramAPI{}, &fakeTrainerClient{})
	bot.clearWorkoutPending(context.Background(), "42")
	bot.clearWorkoutPending(context.Background(), "")
}

func TestPendingCancelKeyboard(t *testing.T) {
	kb := pendingCancelKeyboard()
	if len(kb.InlineKeyboard) != 1 || kb.InlineKeyboard[0][0].Text != btnCancelPending {
		t.Fatalf("%+v", kb)
	}
}

func TestBot_dayDoneNoProgram(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{program: trainerclient.TelegramProgramResponse{TrainerDisplayName: "Anna"}},
		WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), workoutcompletion.NewMemoryPendingStore()))
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID: "x", Data: programDayDoneCallbackData(1, 1),
		From: &tgbotapi.User{ID: 42}, Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if len(api.sent) == 0 {
		t.Fatal("expected message")
	}
}

func TestBot_dayDoneMissingWeekDay(t *testing.T) {
	dayKey := uuid.New()
	prog := programWithAssignment(dayKey)
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{program: prog},
		WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), workoutcompletion.NewMemoryPendingStore()))
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID: "x", Data: programDayDoneCallbackData(9, 1),
		From: &tgbotapi.User{ID: 42}, Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if len(api.sent) == 0 || !strings.Contains(api.sent[0].Text, "не найдена") && !strings.Contains(api.sent[0].Text, "Программа") {
		// formatProgramNotFoundMessage
		if len(api.sent) == 0 {
			t.Fatal("expected not found message")
		}
	}
	api.sent = nil
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID: "y", Data: programDayDoneCallbackData(1, 9),
		From: &tgbotapi.User{ID: 42}, Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if len(api.sent) == 0 {
		t.Fatal("expected not found for missing day")
	}
}

func TestBot_completeAlreadyCompletedConflict(t *testing.T) {
	dayKey := uuid.New()
	prog := programWithAssignment(dayKey)
	pending := workoutcompletion.NewMemoryPendingStore()
	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{
		ClientUserID: uuid.New(), TrainerID: prog.TrainerID,
		ProgramID: prog.Assignment.ProgramID, ProgramVersionID: prog.Assignment.ProgramVersionID,
		ProgramAssignmentID: prog.Assignment.AssignmentID, CompletionCycleID: prog.Assignment.CompletionCycleID,
		DayKey: dayKey, WeekNumber: 1, DayNumber: 1, ProgramName: "Force",
	})
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{program: prog},
		WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{complete: workoutcompletion.ErrAlreadyCompleted}), pending))
	bot.handleMessage(context.Background(), &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1}, From: &tgbotapi.User{ID: 42}, Text: "done",
	})
	if _, ok, _ := pending.Get(context.Background(), "42"); ok {
		t.Fatal("pending cleared")
	}
	if len(api.sent) == 0 {
		t.Fatal("re-render day")
	}
}

func TestBot_completeGenericError(t *testing.T) {
	dayKey := uuid.New()
	prog := programWithAssignment(dayKey)
	pending := workoutcompletion.NewMemoryPendingStore()
	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{
		ClientUserID: uuid.New(), TrainerID: prog.TrainerID,
		ProgramID: prog.Assignment.ProgramID, ProgramVersionID: prog.Assignment.ProgramVersionID,
		ProgramAssignmentID: prog.Assignment.AssignmentID, CompletionCycleID: prog.Assignment.CompletionCycleID,
		DayKey: dayKey, WeekNumber: 1, DayNumber: 1,
	})
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{program: prog},
		WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{complete: errors.New("db down")}), pending))
	bot.handleMessage(context.Background(), &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1}, From: &tgbotapi.User{ID: 42}, Text: "done",
	})
	if len(api.sent) == 0 {
		t.Fatal("expected error message")
	}
}

func TestBot_pendingCancelWithoutPending(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{}, WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), workoutcompletion.NewMemoryPendingStore()))
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID: "c", Data: programDayDoneCancelCallbackData,
		From: &tgbotapi.User{ID: 42}, Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if len(api.sent) == 0 || !strings.Contains(api.sent[0].Text, "Отменено") {
		t.Fatalf("%+v", api.sent)
	}
}

func TestBot_resultDayKeyMismatch(t *testing.T) {
	dayKey := uuid.New()
	prog := programWithAssignment(dayKey)
	pending := workoutcompletion.NewMemoryPendingStore()
	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{
		DayKey: uuid.New(), WeekNumber: 1, DayNumber: 1,
		CompletionCycleID: prog.Assignment.CompletionCycleID,
	})
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{program: prog},
		WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), pending))
	bot.handleMessage(context.Background(), &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1}, From: &tgbotapi.User{ID: 42}, Text: "ok",
	})
	if _, ok, _ := pending.Get(context.Background(), "42"); ok {
		t.Fatal("cleared")
	}
}

func TestBot_resultNoProgram(t *testing.T) {
	pending := workoutcompletion.NewMemoryPendingStore()
	_ = pending.Set(context.Background(), "42", workoutcompletion.Pending{WeekNumber: 1, DayNumber: 1, DayKey: uuid.New()})
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{program: trainerclient.TelegramProgramResponse{TrainerDisplayName: "A"}},
		WithWorkoutCompletions(workoutcompletion.New(&fakeWorkoutStore{}), pending))
	bot.handleMessage(context.Background(), &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1}, From: &tgbotapi.User{ID: 42}, Text: "ok",
	})
	if _, ok, _ := pending.Get(context.Background(), "42"); ok {
		t.Fatal("cleared")
	}
}
