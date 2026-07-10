package telegrambot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

const (
	btnToday    = "📋 Сегодня"
	btnProgram  = "📅 Программа"
	btnTrainers = "👤 Тренеры"
	btnHelp     = "❓ Помощь"
)

func mainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnToday),
			tgbotapi.NewKeyboardButton(btnProgram),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnTrainers),
			tgbotapi.NewKeyboardButton(btnHelp),
		),
	)
}
