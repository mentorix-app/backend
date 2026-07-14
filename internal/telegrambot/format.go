package telegrambot

import (
	"errors"
	"fmt"
	"strings"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

const cyrillicMultiplicationSign = "х"

func formatToday(resp trainerclient.TelegramTodayResponse) string {
	if !resp.HasProgram {
		return fmt.Sprintf("%s пока не назначил программу.", resp.TrainerDisplayName)
	}
	name := programDisplayName(resp.ProgramName, resp.ProgramNameRu)
	var b strings.Builder
	b.WriteString("📋 Сегодня\n")
	fmt.Fprintf(&b, "👤 Тренер: %s\n", escapeTelegramMarkdown(resp.TrainerDisplayName))
	fmt.Fprintf(&b, "💪 Программа: %s\n", escapeTelegramMarkdown(name))
	fmt.Fprintf(&b, "📆 Неделя: %d / День: %d", resp.WeekNumber, resp.DayNumber)
	if resp.IsRestDay {
		b.WriteString("\n\n😴 День отдыха")
		return b.String()
	}
	if blockText := formatBlocks(resp.Blocks); blockText != "" {
		b.WriteString("\n\n")
		b.WriteString(blockText)
	}
	return b.String()
}

func formatProgramSummary(resp trainerclient.TelegramProgramResponse) string {
	if !resp.HasProgram || resp.Program == nil {
		return fmt.Sprintf("%s пока не назначил программу.", resp.TrainerDisplayName)
	}
	name := programDisplayName(resp.Program.Name, resp.Program.NameRu)
	var b strings.Builder
	fmt.Fprintf(&b, "👤 Тренер: %s\n", escapeTelegramMarkdown(resp.TrainerDisplayName))
	fmt.Fprintf(&b, "💪 Программа: %s\n", escapeTelegramMarkdown(name))
	fmt.Fprintf(&b, "📆 Недель: %d\n", countSelectableWeeks(resp.Program.Weeks))
	fmt.Fprintf(&b, "🏋 Тренировочных дней: %d\n\n", countSelectableDays(resp.Program.Weeks))
	b.WriteString("Выберите неделю:")
	return b.String()
}

func formatProgramEmptyTrainingDays(trainerName string) string {
	return fmt.Sprintf("%s пока не добавил тренировочные дни в программу.", trainerName)
}

func formatProgramWeekPicker(weekNumber int) string {
	return fmt.Sprintf("📆 Неделя %d\n\nВыберите день:", weekNumber)
}

func formatProgramDay(weekNumber, dayNumber int, day program.Day) string {
	var b strings.Builder
	fmt.Fprintf(&b, "📆 Неделя %d / День %d", weekNumber, dayNumber)
	if blockText := formatBlocks(day.Blocks); blockText != "" {
		b.WriteString("\n\n")
		b.WriteString(blockText)
	}
	return b.String()
}

func formatProgramNotFoundMessage() string {
	return "Раздел не найден. Откройте «Программа» снова."
}

func formatTrainers(list trainerclient.TelegramTrainerList) (string, []trainerclient.TelegramTrainer) {
	if len(list.Items) == 0 {
		return "👤 Тренеры\n\nУ вас пока нет тренеров. Откройте ссылку-приглашение от тренера.", nil
	}

	var b strings.Builder
	b.WriteString("👤 Тренеры\n")

	if len(list.Items) == 1 {
		b.WriteString("\n")
		writeTrainerCard(&b, list.Items[0], false)
		return b.String(), nil
	}

	for i, t := range list.Items {
		if i == 0 {
			b.WriteString("\n")
		} else {
			b.WriteString("\n\n")
		}
		writeTrainerCard(&b, t, true)
	}
	b.WriteString("\n\nНажмите кнопку ниже, чтобы выбрать активного тренера.")
	return b.String(), list.Items
}

func writeTrainerCard(b *strings.Builder, t trainerclient.TelegramTrainer, showActive bool) {
	if showActive && t.IsActive {
		b.WriteString("✓ ")
	}
	fmt.Fprintf(b, "👤 Тренер: %s\n", escapeTelegramMarkdown(t.DisplayName))
	if t.HasProgram {
		fmt.Fprintf(b, "💪 Программа: %s", escapeTelegramMarkdown(programDisplayName(t.ProgramName, t.ProgramNameRu)))
	} else {
		b.WriteString("💪 Программа не назначена")
	}
}

func formatActiveTrainerNotification(t trainerclient.TelegramTrainer) string {
	var b strings.Builder
	b.WriteString("✓ ")
	writeTrainerCard(&b, t, false)
	return b.String()
}

func formatBlocks(blocks []program.DayBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if text := formatBlock(block); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func formatBlock(block program.DayBlock) string {
	if len(block.Exercises) == 0 && strings.TrimSpace(block.Instruction) == "" {
		return ""
	}

	if blockIsGroup(block.BlockType) {
		var b strings.Builder
		fmt.Fprintf(&b, "%s:\n", blockTypeHeading(block.BlockType))
		if instr := strings.TrimSpace(block.Instruction); instr != "" {
			writeInstructionLine(&b, instr)
		}
		b.WriteString("\n")
		for i, ex := range block.Exercises {
			if i > 0 {
				b.WriteString("\n")
			}
			writeGroupExercise(&b, ex, i)
		}
		return strings.TrimRight(b.String(), "\n")
	}

	parts := make([]string, 0, len(block.Exercises))
	for _, ex := range block.Exercises {
		parts = append(parts, formatStandaloneExercise(ex))
	}
	return strings.Join(parts, "\n\n")
}

func formatStandaloneExercise(ex program.DayExercise) string {
	var b strings.Builder
	b.WriteString("🏋 ")
	writeExerciseLine(&b, ex, "")
	if instr := strings.TrimSpace(ex.Instruction); instr != "" {
		b.WriteString("\n")
		writeInstructionLine(&b, instr)
	}
	return b.String()
}

func writeGroupExercise(b *strings.Builder, ex program.DayExercise, index int) {
	writeExerciseLine(b, ex, exerciseLetterCyrillic(index))
	if instr := strings.TrimSpace(ex.Instruction); instr != "" {
		b.WriteString("\n")
		writeInstructionLine(b, instr)
	}
}

func writeExerciseLine(b *strings.Builder, ex program.DayExercise, letterPrefix string) {
	name := escapeTelegramMarkdown(exerciseDisplayName(ex.ExerciseName, ex.ExerciseNameRu))
	if letterPrefix != "" {
		fmt.Fprintf(b, "%s %s", letterPrefix, name)
	} else {
		b.WriteString(name)
	}
	if vol := formatExerciseVolume(ex); vol != "" {
		b.WriteString(" - " + vol)
	}
}

func writeInstructionLine(b *strings.Builder, instruction string) {
	fmt.Fprintf(b, "_%s_", escapeTelegramMarkdown(instruction))
}

func formatExerciseVolume(ex program.DayExercise) string {
	switch {
	case ex.Sets != nil && ex.Reps != nil:
		return fmt.Sprintf("%s%s%s", *ex.Sets, cyrillicMultiplicationSign, *ex.Reps)
	case ex.Sets != nil:
		return fmt.Sprintf("%s подх.", *ex.Sets)
	case ex.Reps != nil:
		return fmt.Sprintf("%s повт.", *ex.Reps)
	default:
		return ""
	}
}

func exerciseLetterCyrillic(i int) string {
	if i < 32 {
		return string(rune('А'+i)) + "."
	}
	return fmt.Sprintf("%d.", i+1)
}

func blockIsGroup(t program.BlockType) bool {
	return t != "" && t != program.BlockTypeSingle
}

func blockTypeHeading(t program.BlockType) string {
	icon, label := blockTypeIcon(t), blockTypeLabel(t)
	if icon == "" {
		return label
	}
	return icon + " " + label
}

func blockTypeIcon(t program.BlockType) string {
	switch t {
	case program.BlockTypeSuperset:
		return "🔗"
	case program.BlockTypeComplex:
		return "🧩"
	case program.BlockTypeEMOM:
		return "⏱"
	case program.BlockTypeAMRAP:
		return "🔁"
	case program.BlockTypeForTime:
		return "🏁"
	case program.BlockTypeIntervals:
		return "⚡"
	case program.BlockTypeChipper:
		return "📉"
	case program.BlockTypeLadder:
		return "📈"
	case program.BlockTypeDeathBy:
		return "💀"
	default:
		return ""
	}
}

func blockTypeLabel(t program.BlockType) string {
	switch t {
	case program.BlockTypeSuperset:
		return "Суперсет"
	case program.BlockTypeComplex:
		return "Комплекс"
	case program.BlockTypeEMOM:
		return "EMOM"
	case program.BlockTypeAMRAP:
		return "AMRAP"
	case program.BlockTypeForTime:
		return "For Time"
	case program.BlockTypeIntervals:
		return "Интервалы"
	case program.BlockTypeChipper:
		return "Чиппер"
	case program.BlockTypeLadder:
		return "Лестница"
	case program.BlockTypeDeathBy:
		return "Death By"
	default:
		return "Блок"
	}
}

func escapeTelegramMarkdown(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "_", `\_`)
	s = strings.ReplaceAll(s, "*", `\*`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "[", `\[`)
	return s
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
