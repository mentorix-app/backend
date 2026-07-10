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
)

const inviteStartPrefix = "inv_"

type trainerClient interface {
	AcceptInvite(ctx context.Context, req trainerclient.AcceptInviteRequest) (trainerclient.AcceptInviteResult, error)
	RefreshTelegramAvatar(ctx context.Context, telegramUserID string) error
	ListTelegramTrainers(ctx context.Context, telegramUserID string) (trainerclient.TelegramTrainerList, error)
	SetTelegramActiveTrainer(ctx context.Context, telegramUserID string, trainerID uuid.UUID) (*trainerclient.ActiveTrainerResponse, error)
	GetTelegramToday(ctx context.Context, telegramUserID string, trainerID *uuid.UUID) (trainerclient.TelegramTodayResponse, error)
	GetTelegramProgram(ctx context.Context, telegramUserID string, trainerID *uuid.UUID) (trainerclient.TelegramProgramResponse, error)
}

type telegramAPI interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

type Bot struct {
	api     telegramAPI
	clients trainerClient
}

func New(api telegramAPI, clients trainerClient) *Bot {
	return &Bot{api: api, clients: clients}
}

func NewFromToken(token string, clients trainerClient) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot api: %w", err)
	}
	api.Debug = false
	return New(api, clients), nil
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
	case btnToday:
		b.handleToday(ctx, msg.Chat.ID, tgID)
	case btnProgram:
		b.handleProgram(ctx, msg.Chat.ID, tgID)
	case btnTrainers:
		b.handleTrainers(ctx, msg.Chat.ID, tgID)
	case btnHelp:
		b.sendText(msg.Chat.ID, helpText(), mainMenuKeyboard())
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

func (b *Bot) handleToday(ctx context.Context, chatID int64, telegramUserID string) {
	resp, err := b.clients.GetTelegramToday(ctx, telegramUserID, nil)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	b.sendMarkdown(chatID, formatToday(resp), mainMenuKeyboard())
}

func (b *Bot) handleProgram(ctx context.Context, chatID int64, telegramUserID string) {
	resp, err := b.clients.GetTelegramProgram(ctx, telegramUserID, nil)
	if err != nil {
		b.sendText(chatID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	for _, part := range formatProgram(resp) {
		b.sendMarkdown(chatID, part, mainMenuKeyboard())
	}
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
	_, _ = b.api.Request(tgbotapi.NewCallback(query.ID, ""))

	trainerID, ok := parseTrainerCallback(query.Data)
	if !ok {
		return
	}
	tgID := telegramUserID(query.From)
	result, err := b.clients.SetTelegramActiveTrainer(ctx, tgID, trainerID)
	if err != nil {
		b.sendText(query.Message.Chat.ID, menuErrorText(err), mainMenuKeyboard())
		return
	}
	b.sendText(query.Message.Chat.ID, fmt.Sprintf("Активный тренер: %s", result.DisplayName), mainMenuKeyboard())
	b.handleTrainers(ctx, query.Message.Chat.ID, tgID)
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
	case errors.Is(err, trainerclient.ErrInviteNotConfigured):
		return "Бот временно недоступен. Попробуйте позже."
	}
	return "Не удалось принять приглашение. Попробуйте позже."
}

func helpText() string {
	return "Mentorix — программы тренировок от вашего тренера.\n\n" +
		"📋 Сегодня — тренировка на сегодня\n" +
		"📅 Программа — вся программа\n" +
		"👤 Тренеры — ваши тренеры\n\n" +
		"Команды: /menu — показать меню"
}
