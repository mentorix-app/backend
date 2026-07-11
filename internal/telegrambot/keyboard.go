package telegrambot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

const (
	btnProgram  = "📅 Программа"
	btnTrainers = "👤 Тренеры"
	btnHelp     = "❓ Помощь"
)

func mainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnProgram),
			tgbotapi.NewKeyboardButton(btnTrainers),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnHelp),
		),
	)
}
