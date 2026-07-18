package telegrambot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
	"mentorix-backend/internal/workoutcompletion"
)

const inviteStartPrefix = "inv_"

type trainerClient interface {
	AcceptInvite(ctx context.Context, req trainerclient.AcceptInviteRequest) (trainerclient.AcceptInviteResult, error)
	RefreshTelegramAvatar(ctx context.Context, telegramUserID string) error
	ListTelegramTrainers(ctx context.Context, telegramUserID string) (trainerclient.TelegramTrainerList, error)
	SetTelegramActiveTrainer(ctx context.Context, telegramUserID string, trainerID uuid.UUID) (*trainerclient.ActiveTrainerResponse, error)
	GetTelegramProgram(ctx context.Context, telegramUserID string, trainerID *uuid.UUID) (trainerclient.TelegramProgramResponse, error)
	ClientUserIDByTelegram(ctx context.Context, telegramUserID string) (uuid.UUID, error)
}

type telegramAPI interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

type Bot struct {
	api      telegramAPI
	clients  trainerClient
	workouts *workoutcompletion.Service
	pending  workoutcompletion.PendingStore
}

type BotOption func(*Bot)

func WithWorkoutCompletions(svc *workoutcompletion.Service, pending workoutcompletion.PendingStore) BotOption {
	return func(b *Bot) {
		b.workouts = svc
		b.pending = pending
	}
}

func New(api telegramAPI, clients trainerClient, opts ...BotOption) *Bot {
	b := &Bot{api: api, clients: clients}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

func NewFromToken(token string, clients trainerClient, opts ...BotOption) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot api: %w", err)
	}
	api.Debug = false
	return New(api, clients, opts...), nil
}

func (b *Bot) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		b.handleCallbackQuery(ctx, update.CallbackQuery)
		return
	}
	if update.Message == nil {
		return
	}
	b.handleMessage(ctx, update.Message)
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	tgID := telegramUserID(msg.From)
	if tgID != "" {
		_ = b.clients.RefreshTelegramAvatar(ctx, tgID)
	}

	if msg.IsCommand() {
		b.clearWorkoutPending(ctx, tgID)
		switch msg.Command() {
		case "start":
			b.handleStart(ctx, msg)
			return
		case "menu", "help":
			b.sendText(msg.Chat.ID, helpText(), mainMenuKeyboard())
			return
		}
	}

	switch strings.TrimSpace(msg.Text) {
	case btnProgram, btnTrainers, btnHelp:
		b.clearWorkoutPending(ctx, tgID)
	}

	if b.tryHandleWorkoutResult(ctx, msg) {
		return
	}

	switch strings.TrimSpace(msg.Text) {
	case btnProgram:
		b.handleProgram(ctx, msg.Chat.ID, tgID)
		return
	case btnTrainers:
		b.handleTrainers(ctx, msg.Chat.ID, tgID)
		return
	case btnHelp:
		b.sendText(msg.Chat.ID, helpText(), mainMenuKeyboard())
		return
	default:
		if strings.TrimSpace(msg.Text) != "" {
			b.sendText(msg.Chat.ID, "Выберите пункт меню или отправьте /menu.", mainMenuKeyboard())
		}
	}
}

func (b *Bot) handleStart(ctx context.Context, msg *tgbotapi.Message) {
	if token, ok := parseInviteToken(msg.CommandArguments()); ok {
		b.acceptInvite(ctx, msg, token)
		return
	}
	b.sendText(msg.Chat.ID, "Откройте ссылку-приглашение от тренера или выберите пункт меню.", mainMenuKeyboard())
}

func (b *Bot) handleProgram(ctx context.Context, chatID int64, telegramUserID string) {
	resp, err := b.clients.GetTelegramProgram(ctx, telegramUserID, nil)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	if !resp.HasProgram || resp.Program == nil {
		b.sendText(chatID, formatProgramSummary(resp), mainMenuKeyboard())
		return
	}
	var completed map[uuid.UUID]struct{}
	if b.workouts != nil && resp.Assignment != nil {
		var cerr error
		completed, cerr = b.workouts.CompletedDayKeys(ctx, resp.Assignment.CompletionCycleID)
		if cerr != nil {
			b.sendText(chatID, menuErrorText(cerr), mainMenuKeyboard())
			return
		}
	}
	inline := programWeeksKeyboard(resp.Program.Weeks, completed)
	if len(inline.InlineKeyboard) == 0 {
		b.sendText(chatID, formatProgramEmptyTrainingDays(resp.TrainerDisplayName), mainMenuKeyboard())
		return
	}
	b.sendMarkdownWithInline(chatID, formatProgramSummary(resp), mainMenuKeyboard(), inline)
}

func (b *Bot) handleProgramWeek(ctx context.Context, chatID int64, telegramUserID string, weekNumber int) {
	resp, err := b.clients.GetTelegramProgram(ctx, telegramUserID, nil)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	if !resp.HasProgram || resp.Program == nil {
		b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return
	}
	week, ok := findWeek(resp.Program.Weeks, weekNumber)
	if !ok || !weekHasSelectableDays(*week) {
		b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return
	}
	var completed map[uuid.UUID]struct{}
	if b.workouts != nil && resp.Assignment != nil {
		var cerr error
		completed, cerr = b.workouts.CompletedDayKeys(ctx, resp.Assignment.CompletionCycleID)
		if cerr != nil {
			b.sendText(chatID, menuErrorText(cerr), mainMenuKeyboard())
			return
		}
	}
	inline := programDaysKeyboard(weekNumber, week.Days, completed)
	if len(inline.InlineKeyboard) == 0 {
		b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return
	}
	b.sendMarkdownWithInline(chatID, formatProgramWeekPicker(weekNumber), mainMenuKeyboard(), inline)
}

func (b *Bot) handleProgramDay(ctx context.Context, chatID int64, telegramUserID string, weekNumber, dayNumber int) {
	resp, err := b.clients.GetTelegramProgram(ctx, telegramUserID, nil)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	if !resp.HasProgram || resp.Program == nil {
		b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return
	}
	week, ok := findWeek(resp.Program.Weeks, weekNumber)
	if !ok {
		b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return
	}
	day, ok := findDay(week.Days, dayNumber)
	if !ok || !dayHasExercises(*day) {
		b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
		return
	}
	completed := false
	if b.workouts != nil && resp.Assignment != nil {
		completed, err = b.workouts.IsCompleted(ctx, resp.Assignment.CompletionCycleID, day.DayKey)
		if err != nil {
			b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
			return
		}
	}
	b.sendProgramDay(chatID, weekNumber, dayNumber, *day, resp.Program.Weeks, completed)
}

func (b *Bot) sendProgramDay(chatID int64, weekNumber, dayNumber int, day program.Day, weeks []program.Week, completed bool) {
	text := formatProgramDay(weekNumber, dayNumber, day)
	inline := programDayKeyboard(weeks, weekNumber, dayNumber, completed)
	b.sendMarkdownWithInline(chatID, text, mainMenuKeyboard(), inline)
}

func (b *Bot) handleTrainers(ctx context.Context, chatID int64, telegramUserID string) {
	list, err := b.clients.ListTelegramTrainers(ctx, telegramUserID)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	text, trainers := formatTrainers(list)
	inline := trainersInlineKeyboard(trainers)
	if len(inline.InlineKeyboard) == 0 {
		b.sendMarkdown(chatID, text, mainMenuKeyboard())
		return
	}
	b.sendMarkdownWithInline(chatID, text, mainMenuKeyboard(), inline)
}

func (b *Bot) handleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) {
	if query == nil || query.Message == nil {
		return
	}

	tgID := telegramUserID(query.From)
	chatID := query.Message.Chat.ID

	if query.Data == programDayDoneCancelCallbackData {
		b.answerCallback(query.ID, "")
		b.handlePendingCancel(ctx, chatID, tgID)
		return
	}

	// Any other button clears pending wait for result text.
	if weekNum, dayNum, ok := parseProgramDayDoneCallback(query.Data); ok {
		b.handleProgramDayDone(ctx, chatID, tgID, weekNum, dayNum, query.ID)
		return
	}

	b.clearWorkoutPending(ctx, tgID)
	_, _ = b.api.Request(tgbotapi.NewCallback(query.ID, ""))

	if trainerID, ok := parseTrainerCallback(query.Data); ok {
		b.handleTrainerCallback(ctx, chatID, tgID, trainerID)
		return
	}
	if weekNum, ok := parseProgramWeekCallback(query.Data); ok {
		b.handleProgramWeek(ctx, chatID, tgID, weekNum)
		return
	}
	if weekNum, dayNum, ok := parseProgramDayCallback(query.Data); ok {
		b.handleProgramDay(ctx, chatID, tgID, weekNum, dayNum)
	}
}

func (b *Bot) handleTrainerCallback(ctx context.Context, chatID int64, tgID string, trainerID uuid.UUID) {
	_, err := b.clients.SetTelegramActiveTrainer(ctx, tgID, trainerID)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	list, err := b.clients.ListTelegramTrainers(ctx, tgID)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	for _, t := range list.Items {
		if t.TrainerID == trainerID {
			b.sendMarkdown(chatID, formatActiveTrainerNotification(t), mainMenuKeyboard())
			return
		}
	}
	b.sendText(chatID, formatProgramNotFoundMessage(), mainMenuKeyboard())
}

func (b *Bot) acceptInvite(ctx context.Context, msg *tgbotapi.Message, token string) {
	result, err := b.clients.AcceptInvite(ctx, trainerclient.AcceptInviteRequest{
		Token:          token,
		TelegramUserID: telegramUserID(msg.From),
		DisplayName:    displayName(msg.From),
	})
	if err != nil {
		b.sendText(msg.Chat.ID, inviteErrorText(err), mainMenuKeyboard())
		return
	}

	text := welcomeMessage(result)
	list, _ := b.clients.ListTelegramTrainers(ctx, telegramUserID(msg.From))
	if len(list.Items) > 1 && !result.AlreadyLinked {
		text += "\n\nСменить активного тренера можно в «Тренеры»."
	}
	b.sendText(msg.Chat.ID, text, mainMenuKeyboard())
}

func welcomeMessage(result trainerclient.AcceptInviteResult) string {
	if result.AlreadyLinked {
		return fmt.Sprintf("Вы уже подключены к тренеру %s.", result.TrainerDisplayName)
	}
	return fmt.Sprintf("Вы подключены к тренеру %s.\n\nТренер скоро назначит программу — мы пришлём уведомление.", result.TrainerDisplayName)
}

func (b *Bot) sendText(chatID int64, text string, keyboard tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	if _, err := b.api.Send(msg); err != nil {
		_ = err
	}
}

func (b *Bot) sendMarkdown(chatID int64, text string, keyboard tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard
	if _, err := b.api.Send(msg); err != nil {
		b.sendText(chatID, text, keyboard)
	}
}

func (b *Bot) sendMarkdownWithInline(chatID int64, text string, keyboard tgbotapi.ReplyKeyboardMarkup, inline tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = inline
	if _, err := b.api.Send(msg); err != nil {
		b.sendText(chatID, text, keyboard)
		return
	}
}

func parseInviteToken(arg string) (string, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", false
	}
	if !strings.HasPrefix(arg, inviteStartPrefix) {
		return "", false
	}
	token := strings.TrimPrefix(arg, inviteStartPrefix)
	if token == "" {
		return "", false
	}
	return token, true
}

func telegramUserID(user *tgbotapi.User) string {
	if user == nil {
		return ""
	}
	return strconv.FormatInt(user.ID, 10)
}

func displayName(user *tgbotapi.User) string {
	if user == nil {
		return "Client"
	}
	parts := make([]string, 0, 2)
	if n := strings.TrimSpace(user.FirstName); n != "" {
		parts = append(parts, n)
	}
	if n := strings.TrimSpace(user.LastName); n != "" {
		parts = append(parts, n)
	}
	if len(parts) == 0 {
		return "Client"
	}
	return strings.Join(parts, " ")
}

func inviteErrorText(err error) string {
	switch {
	case errors.Is(err, trainerclient.ErrInviteNotFound):
		return "Ссылка не найдена. Попросите новую у тренера."
	case errors.Is(err, trainerclient.ErrInviteExpired):
		return "Ссылка устарела. Попросите новую у тренера."
	case errors.Is(err, trainerclient.ErrInviteConsumed):
		return "Ссылка уже использована. Попросите новую у тренера."
	case errors.Is(err, program.ErrClientBlocked):
		return "Тренер ограничил доступ. Свяжитесь с тренером."
	case errors.Is(err, trainerclient.ErrClientLimitReached):
		return "У тренера сейчас нет свободных мест. Сообщите тренеру — он освободит место или расширит тариф."
	case errors.Is(err, trainerclient.ErrSelfInvite):
		return "Это ваша собственная ссылка-приглашение: подключиться к самому себе нельзя."
	case errors.Is(err, trainerclient.ErrInviteNotConfigured):
		return "Бот временно недоступен. Попробуйте позже."
	}
	return "Не удалось принять приглашение. Попробуйте позже."
}

func helpText() string {
	return "Mentorix — программы тренировок от вашего тренера.\n\n" +
		"📅 Программа — выбор недели и дня\n" +
		"👤 Тренеры — ваши тренеры\n\n" +
		"Команды: /menu — показать меню"
}
