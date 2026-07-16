package telegrambot

import (
	"context"
	"net/http"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

func TestParseInviteToken(t *testing.T) {
	token, ok := parseInviteToken("inv_abc123")
	if !ok || token != "abc123" {
		t.Fatalf("token = %q ok=%v", token, ok)
	}
	if _, ok := parseInviteToken(""); ok {
		t.Fatal("expected false for empty")
	}
	if _, ok := parseInviteToken("inv_"); ok {
		t.Fatal("expected false for inv_ only")
	}
	if _, ok := parseInviteToken("bad_abc"); ok {
		t.Fatal("expected false for wrong prefix")
	}
}

func TestDisplayName(t *testing.T) {
	if displayName(nil) != "Client" {
		t.Fatal("nil user")
	}
	name := displayName(&tgbotapi.User{FirstName: "Ivan", LastName: "Petrov"})
	if name != "Ivan Petrov" {
		t.Fatalf("name = %q", name)
	}
}

func TestTelegramUserID(t *testing.T) {
	if telegramUserID(nil) != "" {
		t.Fatal("nil user")
	}
	if telegramUserID(&tgbotapi.User{ID: 42}) != "42" {
		t.Fatal("expected 42")
	}
}

func TestWelcomeMessage(t *testing.T) {
	newClient := welcomeMessage(trainerclient.AcceptInviteResult{TrainerDisplayName: "Anna"})
	if newClient == "" {
		t.Fatal("empty welcome")
	}
	linked := welcomeMessage(trainerclient.AcceptInviteResult{TrainerDisplayName: "Anna", AlreadyLinked: true})
	if linked == "" {
		t.Fatal("empty linked")
	}
}

func TestHelpText(t *testing.T) {
	if helpText() == "" {
		t.Fatal("expected help text")
	}
}

func TestInviteErrorText(t *testing.T) {
	cases := []struct {
		err error
		sub string
	}{
		{trainerclient.ErrInviteNotFound, "не найдена"},
		{trainerclient.ErrInviteExpired, "устарела"},
		{trainerclient.ErrInviteConsumed, "использована"},
		{program.ErrClientBlocked, "ограничил"},
		{trainerclient.ErrInviteNotConfigured, "недоступен"},
		{http.ErrServerClosed, "Попробуйте позже"},
	}
	for _, tc := range cases {
		got := inviteErrorText(tc.err)
		if got == "" || !strings.Contains(got, tc.sub) {
			t.Fatalf("inviteErrorText(%v) = %q, want substring %q", tc.err, got, tc.sub)
		}
	}
}

func TestMainMenuKeyboard_buttons(t *testing.T) {
	kb := mainMenuKeyboard()
	if len(kb.Keyboard) != 2 {
		t.Fatalf("rows = %d", len(kb.Keyboard))
	}
	if len(kb.Keyboard[0]) != 2 || len(kb.Keyboard[1]) != 1 {
		t.Fatalf("unexpected layout: %+v", kb.Keyboard)
	}
}

type fakeTelegramAPI struct {
	sent []tgbotapi.MessageConfig
}

func (f *fakeTelegramAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	msg, ok := c.(tgbotapi.MessageConfig)
	if ok {
		f.sent = append(f.sent, msg)
	}
	return tgbotapi.Message{}, nil
}

func (f *fakeTelegramAPI) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

type fakeTrainerClient struct {
	result       trainerclient.AcceptInviteResult
	err          error
	calls        []trainerclient.AcceptInviteRequest
	trainers     trainerclient.TelegramTrainerList
	trainersErr  error
	programErr   error
	program      trainerclient.TelegramProgramResponse
	clientUserID uuid.UUID
	clientUserEr error
}

func (f *fakeTrainerClient) AcceptInvite(_ context.Context, req trainerclient.AcceptInviteRequest) (trainerclient.AcceptInviteResult, error) {
	f.calls = append(f.calls, req)
	return f.result, f.err
}

func (f *fakeTrainerClient) RefreshTelegramAvatar(context.Context, string) error {
	return nil
}

func (f *fakeTrainerClient) ListTelegramTrainers(context.Context, string) (trainerclient.TelegramTrainerList, error) {
	if f.trainersErr != nil {
		return trainerclient.TelegramTrainerList{}, f.trainersErr
	}
	if len(f.trainers.Items) > 0 {
		return f.trainers, nil
	}
	return trainerclient.TelegramTrainerList{Items: []trainerclient.TelegramTrainer{}}, nil
}

func (f *fakeTrainerClient) SetTelegramActiveTrainer(_ context.Context, _ string, id uuid.UUID) (*trainerclient.ActiveTrainerResponse, error) {
	return &trainerclient.ActiveTrainerResponse{TrainerID: id, DisplayName: "Anna"}, nil
}

func (f *fakeTrainerClient) GetTelegramProgram(context.Context, string, *uuid.UUID) (trainerclient.TelegramProgramResponse, error) {
	if f.programErr != nil {
		return trainerclient.TelegramProgramResponse{}, f.programErr
	}
	if f.program.TrainerDisplayName != "" || f.program.Program != nil {
		return f.program, nil
	}
	sets, reps := "3", "10"
	ex := program.DayExercise{ExerciseNameRu: "Присед", Sets: &sets, Reps: &reps}
	return trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		Program: &program.Detail{
			Program: program.Program{NameRu: "Сила"},
			Weeks: []program.Week{{
				WeekNumber: 1,
				Days: []program.Day{
					{DayNumber: 1, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
					{DayNumber: 2, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
				},
			}},
		},
	}, nil
}

func (f *fakeTrainerClient) ClientUserIDByTelegram(context.Context, string) (uuid.UUID, error) {
	if f.clientUserEr != nil {
		return uuid.Nil, f.clientUserEr
	}
	if f.clientUserID != uuid.Nil {
		return f.clientUserID, nil
	}
	return uuid.MustParse("11111111-1111-1111-1111-111111111111"), nil
}

func commandMessage(command, args string) *tgbotapi.Message {
	text := "/" + command
	if args != "" {
		text += " " + args
	}
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		From: &tgbotapi.User{ID: 42, FirstName: "Ivan"},
		Text: text,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: len(command) + 1},
		},
	}
}

func TestBot_handleStart_withoutInvite(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})
	bot.handleStart(context.Background(), commandMessage("start", ""))
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
}

func TestBot_handleStart_acceptInvite(t *testing.T) {
	api := &fakeTelegramAPI{}
	backend := &fakeTrainerClient{result: trainerclient.AcceptInviteResult{TrainerDisplayName: "Anna"}}
	bot := New(api, backend)
	bot.handleStart(context.Background(), commandMessage("start", "inv_tok123"))
	if len(backend.calls) != 1 || backend.calls[0].Token != "tok123" {
		t.Fatalf("calls = %+v", backend.calls)
	}
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
}

func TestBot_handleStart_acceptInvite_error(t *testing.T) {
	api := &fakeTelegramAPI{}
	backend := &fakeTrainerClient{err: trainerclient.ErrInviteExpired}
	bot := New(api, backend)
	bot.handleStart(context.Background(), commandMessage("start", "inv_tok123"))
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
}

func TestBot_handleMessage_menuButtons(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})

	for _, text := range []string{btnProgram, btnTrainers, btnHelp, "random text"} {
		api.sent = nil
		bot.handleMessage(context.Background(), &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 1},
			From: &tgbotapi.User{ID: 42},
			Text: text,
		})
		if len(api.sent) != 1 {
			t.Fatalf("text %q: sent = %d", text, len(api.sent))
		}
	}

	api.sent = nil
	bot.handleMessage(context.Background(), &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}, Text: "   "})
	if len(api.sent) != 0 {
		t.Fatal("expected no reply for whitespace")
	}
}

func TestBot_handleMessage_commands(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})

	for _, cmd := range []string{"menu", "help"} {
		api.sent = nil
		bot.handleMessage(context.Background(), commandMessage(cmd, ""))
		if len(api.sent) != 1 {
			t.Fatalf("cmd %q: sent = %d", cmd, len(api.sent))
		}
	}
}

func TestBot_handleProgram_noProgram(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{
		program: trainerclient.TelegramProgramResponse{
			TrainerDisplayName: "Anna",
			HasProgram:         false,
		},
	})
	bot.handleProgram(context.Background(), 1, "42")
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "не назначил программу") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestBot_handleProgram_emptyTrainingDays(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{
		program: trainerclient.TelegramProgramResponse{
			TrainerDisplayName: "Anna",
			HasProgram:         true,
			Program: &program.Detail{
				Program: program.Program{NameRu: "Сила"},
				Weeks:   []program.Week{{WeekNumber: 1, Days: []program.Day{{DayNumber: 1}}}},
			},
		},
	})
	bot.handleProgram(context.Background(), 1, "42")
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "тренировочные дни") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestBot_handleProgramWeek_invalidWeek(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})
	bot.handleProgramWeek(context.Background(), 1, "42", 99)
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "не найден") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestBot_handleProgramDay_invalidDay(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})
	bot.handleProgramDay(context.Background(), 1, "42", 1, 99)
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "не найден") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestBot_handleProgram(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})
	bot.handleProgram(context.Background(), 1, "42")
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
}

func TestBot_handleCallbackQuery_programWeekAndDay(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})
	chat := &tgbotapi.Chat{ID: 1}
	user := &tgbotapi.User{ID: 42}

	api.sent = nil
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "cb-week",
		Data:    programWeekCallbackData(1),
		From:    user,
		Message: &tgbotapi.Message{Chat: chat},
	})
	if len(api.sent) != 1 {
		t.Fatalf("week sent = %d", len(api.sent))
	}

	api.sent = nil
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "cb-day",
		Data:    programDayCallbackData(1, 1),
		From:    user,
		Message: &tgbotapi.Message{Chat: chat},
	})
	if len(api.sent) != 1 {
		t.Fatalf("day sent = %d", len(api.sent))
	}
	if !strings.Contains(api.sent[0].Text, "Присед") {
		t.Fatalf("text = %q", api.sent[0].Text)
	}
	inline, ok := api.sent[0].ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok || len(inline.InlineKeyboard) < 2 {
		t.Fatalf("reply markup = %+v", api.sent[0].ReplyMarkup)
	}
	if inline.InlineKeyboard[0][0].Text != btnCompleteWorkout {
		t.Fatalf("complete btn = %q", inline.InlineKeyboard[0][0].Text)
	}
	if inline.InlineKeyboard[1][0].Text != "Следующий день" {
		t.Fatalf("nav btn = %q", inline.InlineKeyboard[1][0].Text)
	}

	api.sent = nil
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "cb-next-day",
		Data:    programDayCallbackData(1, 2),
		From:    user,
		Message: &tgbotapi.Message{Chat: chat},
	})
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "День 2") {
		t.Fatalf("next day sent = %+v", api.sent)
	}
	inline, ok = api.sent[0].ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok || len(inline.InlineKeyboard) != 1 || inline.InlineKeyboard[0][0].Text != btnCompleteWorkout {
		t.Fatalf("last day should only have complete btn, got %+v", inline)
	}
}

func TestBot_handleTrainers_multi(t *testing.T) {
	api := &fakeTelegramAPI{}
	backend := &fakeTrainerClient{}
	backend.trainers = trainerclient.TelegramTrainerList{
		Items: []trainerclient.TelegramTrainer{
			{TrainerID: uuid.New(), DisplayName: "Anna"},
			{TrainerID: uuid.New(), DisplayName: "Ivan"},
		},
	}
	bot := New(api, backend)
	bot.handleTrainers(context.Background(), 1, "42")
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
}

func TestBot_handleCallbackQuery_setTrainer(t *testing.T) {
	api := &fakeTelegramAPI{}
	trainerID := uuid.New()
	backend := &fakeTrainerClient{
		trainers: trainerclient.TelegramTrainerList{
			Items: []trainerclient.TelegramTrainer{
				{TrainerID: trainerID, DisplayName: "Anna", HasProgram: true, ProgramNameRu: "Сила"},
				{TrainerID: uuid.New(), DisplayName: "Ivan"},
			},
		},
	}
	bot := New(api, backend)
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:   "cb1",
		Data: trainerCallbackData(trainerID),
		From: &tgbotapi.User{ID: 42},
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 1},
		},
	})
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
	for _, part := range []string{"✓ 👤 Тренер: Anna", "💪 Программа: Сила"} {
		if !strings.Contains(api.sent[0].Text, part) {
			t.Fatalf("missing %q in %q", part, api.sent[0].Text)
		}
	}
	if strings.Contains(api.sent[0].Text, "Нажмите кнопку") {
		t.Fatalf("should not show trainer picker again: %q", api.sent[0].Text)
	}
}

func TestBot_acceptInvite_showsTrainerSwitchHint(t *testing.T) {
	api := &fakeTelegramAPI{}
	backend := &fakeTrainerClient{
		result: trainerclient.AcceptInviteResult{TrainerDisplayName: "Anna"},
		trainers: trainerclient.TelegramTrainerList{
			Items: []trainerclient.TelegramTrainer{
				{TrainerID: uuid.New(), DisplayName: "Anna"},
				{TrainerID: uuid.New(), DisplayName: "Ivan"},
			},
		},
	}
	bot := New(api, backend)
	bot.acceptInvite(context.Background(), &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 42, FirstName: "Ivan"},
	}, "tok")
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "Тренеры") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestBot_handleCallbackQuery_invalidData(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{})
	bot.handleCallbackQuery(context.Background(), &tgbotapi.CallbackQuery{
		ID:      "cb1",
		Data:    "unknown",
		From:    &tgbotapi.User{ID: 42},
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 1}},
	})
	if len(api.sent) != 0 {
		t.Fatalf("sent = %d", len(api.sent))
	}
}

func TestBot_handleProgram_error(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{programErr: http.ErrServerClosed})
	bot.handleProgram(context.Background(), 1, "42")
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
}

func TestBot_handleProgram_activeTrainerNotSet(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{programErr: trainerclient.ErrActiveTrainerNotSet})
	bot.handleProgram(context.Background(), 1, "42")
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].Text, "Тренеры") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestBot_handleTrainers_withoutActiveStillShowsPicker(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{
		trainers: trainerclient.TelegramTrainerList{
			Items: []trainerclient.TelegramTrainer{
				{TrainerID: uuid.New(), DisplayName: "Anna"},
				{TrainerID: uuid.New(), DisplayName: "Ivan"},
			},
		},
	})
	bot.handleTrainers(context.Background(), 1, "42")
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
	if !strings.Contains(api.sent[0].Text, "Anna") || !strings.Contains(api.sent[0].Text, "Ivan") {
		t.Fatalf("text = %q", api.sent[0].Text)
	}
	markup, ok := api.sent[0].ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok || len(markup.InlineKeyboard) != 2 {
		t.Fatalf("inline = %+v", api.sent[0].ReplyMarkup)
	}
}

func TestBot_handleTrainers_error(t *testing.T) {
	api := &fakeTelegramAPI{}
	bot := New(api, &fakeTrainerClient{trainersErr: http.ErrServerClosed})
	bot.handleTrainers(context.Background(), 1, "42")
	if len(api.sent) != 1 {
		t.Fatalf("sent = %d", len(api.sent))
	}
}

func TestDisplayName_lastOnly(t *testing.T) {
	name := displayName(&tgbotapi.User{LastName: "Petrov"})
	if name != "Petrov" {
		t.Fatalf("name = %q", name)
	}
}
