package telegramnotify

import (
	"strconv"
	"strings"
)

// maxResultQuoteLen caps the quoted client result inside the comment
// notification so the whole message stays well under Telegram's 4096-char limit.
const maxResultQuoteLen = 500

func programDisplayName(name, nameRu string) string {
	if strings.TrimSpace(nameRu) != "" {
		return strings.TrimSpace(nameRu)
	}
	return strings.TrimSpace(name)
}

func assignedMessage(trainerName, programName string) string {
	return "Тренер " + trainerName + " назначил вам программу «" + programName + "».\n\nОткройте «Программа» в меню."
}

func syncedMessage(trainerName, programName string) string {
	return "Тренер " + trainerName + " обновил вашу программу «" + programName + "».\n\nОткройте «Программа» в меню, чтобы посмотреть изменения."
}

func workoutCommentMessage(trainerName string, c WorkoutComment) string {
	programName := programDisplayName(c.ProgramName, c.ProgramNameRu)
	if programName == "" {
		programName = "программа"
	}
	return "Тренер " + trainerName + " ответил на ваш результат тренировки «" + programName +
		"», неделя " + strconv.Itoa(c.WeekNumber) + ", день " + strconv.Itoa(c.DayNumber) + ".\n\n" +
		"Ваш результат:\n" + truncateRunes(strings.TrimSpace(c.ResultText), maxResultQuoteLen) + "\n\n" +
		"Ответ тренера:\n" + c.CommentText
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}
