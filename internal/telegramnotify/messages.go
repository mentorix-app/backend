package telegramnotify

import "strings"

func programDisplayName(name, nameRu string) string {
	if strings.TrimSpace(nameRu) != "" {
		return strings.TrimSpace(nameRu)
	}
	return strings.TrimSpace(name)
}

func assignedMessage(trainerName, programName string) string {
	return "Тренер " + trainerName + " назначил вам программу «" + programName + "».\n\nОткройте «Сегодня» или «Программа» в меню."
}

func syncedMessage(trainerName, programName string) string {
	return "Тренер " + trainerName + " обновил вашу программу «" + programName + "».\n\nОткройте «Программа» в меню, чтобы посмотреть изменения."
}
