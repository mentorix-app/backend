package telegrambot

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"mentorix-backend/internal/trainerclient"
)

const trainerCallbackPrefix = "active_trainer:"

func trainerCallbackData(trainerID uuid.UUID) string {
	return trainerCallbackPrefix + trainerID.String()
}

func parseTrainerCallback(data string) (uuid.UUID, bool) {
	if !strings.HasPrefix(data, trainerCallbackPrefix) {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimPrefix(data, trainerCallbackPrefix))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func trainersInlineKeyboard(trainers []trainerclient.TelegramTrainer) tgbotapi.InlineKeyboardMarkup {
	if len(trainers) <= 1 {
		return tgbotapi.InlineKeyboardMarkup{}
	}
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(trainers))
	for _, t := range trainers {
		label := t.DisplayName
		if t.IsActive {
			label = "✓ " + label
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, trainerCallbackData(t.TrainerID)),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
