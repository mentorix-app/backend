package telegrambot

import (
	"errors"
	"fmt"
	"strings"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

const telegramMessageLimit = 4000

func formatToday(resp trainerclient.TelegramTodayResponse) string {
	if !resp.HasProgram {
		return fmt.Sprintf("%s пока не назначил программу.", resp.TrainerDisplayName)
	}
	name := programDisplayName(resp.ProgramName, resp.ProgramNameRu)
	var b strings.Builder
	fmt.Fprintf(&b, "📋 *Сегодня*\nТренер: %s\nПрограмма: %s\nДень %d (неделя %d, день %d)\n",
		resp.TrainerDisplayName, name, resp.ProgramDayNumber, resp.WeekNumber, resp.DayNumber)
	if resp.IsRestDay {
		b.WriteString("\nСегодня день отдыха.")
		return b.String()
	}
	b.WriteString("\n")
	b.WriteString(formatBlocks(resp.Blocks))
	return b.String()
}

func formatProgram(resp trainerclient.TelegramProgramResponse) []string {
	if !resp.HasProgram || resp.Program == nil {
		return []string{fmt.Sprintf("%s пока не назначил программу.", resp.TrainerDisplayName)}
	}
	name := programDisplayName(resp.Program.Name, resp.Program.NameRu)
	header := fmt.Sprintf("📅 *Программа*\nТренер: %s\n%s\n", resp.TrainerDisplayName, name)

	var parts []string
	var current strings.Builder
	current.WriteString(header)

	for _, week := range resp.Program.Weeks {
		weekHeader := fmt.Sprintf("\n*Неделя %d*\n", week.WeekNumber)
		if current.Len()+len(weekHeader) > telegramMessageLimit {
			parts = append(parts, current.String())
			current.Reset()
		}
		current.WriteString(weekHeader)
		for _, day := range week.Days {
			dayHeader := fmt.Sprintf("День %d\n", day.DayNumber)
			if current.Len()+len(dayHeader) > telegramMessageLimit {
				parts = append(parts, current.String())
				current.Reset()
			}
			current.WriteString(dayHeader)
			blockText := formatBlocks(day.Blocks)
			if blockText == "" {
				blockText = "— отдых\n"
			}
			if current.Len()+len(blockText) > telegramMessageLimit {
				parts = append(parts, current.String())
				current.Reset()
			}
			current.WriteString(blockText)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	if len(parts) == 0 {
		return []string{header + "\nПрограмма пуста."}
	}
	return parts
}

func formatTrainers(list trainerclient.TelegramTrainerList) (string, []trainerclient.TelegramTrainer) {
	if len(list.Items) == 0 {
		return "У вас пока нет тренеров. Откройте ссылку-приглашение от тренера.", nil
	}
	if len(list.Items) == 1 {
		t := list.Items[0]
		var b strings.Builder
		fmt.Fprintf(&b, "👤 *%s*\n", t.DisplayName)
		if t.HasProgram {
			b.WriteString("Программа: " + programDisplayName(t.ProgramName, t.ProgramNameRu))
		} else {
			b.WriteString("Программа пока не назначена.")
		}
		return b.String(), nil
	}

	var b strings.Builder
	b.WriteString("👤 *Ваши тренеры*\n\n")
	for _, t := range list.Items {
		mark := "  "
		if t.IsActive {
			mark = "✓ "
		}
		line := fmt.Sprintf("%s%s", mark, t.DisplayName)
		if t.HasProgram {
			line += " — " + programDisplayName(t.ProgramName, t.ProgramNameRu)
		} else {
			line += " — программа не назначена"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nНажмите кнопку ниже, чтобы выбрать активного тренера.")
	return b.String(), list.Items
}

func formatBlocks(blocks []program.DayBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	var b strings.Builder
	for _, block := range blocks {
		if block.Instruction != "" {
			fmt.Fprintf(&b, "_%s_\n", block.Instruction)
		}
		for _, ex := range block.Exercises {
			name := exerciseDisplayName(ex.ExerciseName, ex.ExerciseNameRu)
			line := "• " + name
			if ex.Sets != nil && ex.Reps != nil {
				line += fmt.Sprintf(" — %d×%d", *ex.Sets, *ex.Reps)
			} else if ex.Sets != nil {
				line += fmt.Sprintf(" — %d подходов", *ex.Sets)
			} else if ex.Reps != nil {
				line += fmt.Sprintf(" — %d повторений", *ex.Reps)
			}
			if ex.Instruction != "" {
				line += fmt.Sprintf(" (%s)", ex.Instruction)
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func programDisplayName(name, nameRu string) string {
	if strings.TrimSpace(nameRu) != "" {
		return strings.TrimSpace(nameRu)
	}
	return strings.TrimSpace(name)
}

func exerciseDisplayName(name, nameRu string) string {
	return programDisplayName(name, nameRu)
}

func menuErrorText(err error) string {
	if errors.Is(err, trainerclient.ErrActiveTrainerNotSet) {
		return "Выберите тренера в разделе «Тренеры»."
	}
	return "Не удалось загрузить данные. Попробуйте позже."
}
