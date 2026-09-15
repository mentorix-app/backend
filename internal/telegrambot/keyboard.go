package telegrambot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

const (
	btnProgram  = "📅 Программа"
	btnTrainers = "👤 Тренеры"
	btnHelp     = "❓ Помощь"
	btnStats    = "📊 Статистика"
)

// mainMenuKeyboard builds the reply keyboard; the stats button is shown only
// when the client analytics page is configured.
func mainMenuKeyboard(withStats bool) tgbotapi.ReplyKeyboardMarkup {
	bottom := []tgbotapi.KeyboardButton{tgbotapi.NewKeyboardButton(btnHelp)}
	if withStats {
		bottom = append(bottom, tgbotapi.NewKeyboardButton(btnStats))
	}
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnProgram),
			tgbotapi.NewKeyboardButton(btnTrainers),
		),
		tgbotapi.NewKeyboardButtonRow(bottom...),
	)
}
