package telegrambot

import (
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

const (
	trainerCallbackPrefix     = "active_trainer:"
	programWeekCallbackPrefix = "program_week:"
	programDayCallbackPrefix  = "program_day:"
)

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

func programWeekCallbackData(weekNumber int) string {
	return fmt.Sprintf("%s%d", programWeekCallbackPrefix, weekNumber)
}

func programDayCallbackData(weekNumber, dayNumber int) string {
	return fmt.Sprintf("%s%d:%d", programDayCallbackPrefix, weekNumber, dayNumber)
}

func parseProgramWeekCallback(data string) (int, bool) {
	if !strings.HasPrefix(data, programWeekCallbackPrefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(data, programWeekCallbackPrefix))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func parseProgramDayCallback(data string) (weekNum, dayNum int, ok bool) {
	if !strings.HasPrefix(data, programDayCallbackPrefix) {
		return 0, 0, false
	}
	rest := strings.TrimPrefix(data, programDayCallbackPrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	weekNum, err := strconv.Atoi(parts[0])
	if err != nil || weekNum <= 0 {
		return 0, 0, false
	}
	dayNum, err = strconv.Atoi(parts[1])
	if err != nil || dayNum <= 0 {
		return 0, 0, false
	}
	return weekNum, dayNum, true
}

func programWeeksKeyboard(weeks []program.Week, completed map[uuid.UUID]struct{}) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(weeks))
	for _, w := range weeks {
		if !weekHasSelectableDays(w) {
			continue
		}
		label := fmt.Sprintf("Неделя %d", w.WeekNumber)
		if weekFullyCompleted(w, completed) {
			label = "✅ " + label
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				label,
				programWeekCallbackData(w.WeekNumber),
			),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func programDaysKeyboard(weekNumber int, days []program.Day, completed map[uuid.UUID]struct{}) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(days))
	for _, d := range days {
		if !dayHasExercises(d) {
			continue
		}
		label := fmt.Sprintf("День %d", d.DayNumber)
		if _, ok := completed[d.DayKey]; ok {
			label = "✅ " + label
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				label,
				programDayCallbackData(weekNumber, d.DayNumber),
			),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func programDayNavKeyboard(weeks []program.Week, weekNumber, dayNumber int) tgbotapi.InlineKeyboardMarkup {
	week, ok := findWeek(weeks, weekNumber)
	if !ok {
		return tgbotapi.InlineKeyboardMarkup{}
	}
	if nextDay, ok := nextSelectableDayInWeek(*week, dayNumber); ok {
		return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"Следующий день",
				programDayCallbackData(weekNumber, nextDay),
			),
		))
	}
	if nextWeek, ok := nextSelectableWeek(weeks, weekNumber); ok {
		return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"Следующая неделя",
				programWeekCallbackData(nextWeek),
			),
		))
	}
	return tgbotapi.InlineKeyboardMarkup{}
}
