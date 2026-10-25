package telegrambot

import (
	"errors"
	"fmt"
	"strings"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

const cyrillicMultiplicationSign = "х"

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
		if line := blockSettingsLine(block); line != "" {
			fmt.Fprintf(&b, "*%s*\n", line)
		}
		if instr := strings.TrimSpace(block.Instruction); instr != "" {
			writeInstructionLine(&b, instr)
			b.WriteString("\n")
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

// blockSettingsLine renders the block format settings joined with " · ",
// for example "8 раундов · работа 20 с / отдых 10 с". It returns an empty
// string when no setting is set. The text is built from constants and
// integers, so it contains no Markdown special characters and needs no
// escaping inside the bold markers.
func blockSettingsLine(block program.DayBlock) string {
	var parts []string
	if block.Rounds != nil {
		parts = append(parts, roundsText(*block.Rounds))
	}
	if block.DurationSec != nil {
		parts = append(parts, formatDurationSec(*block.DurationSec))
	}
	if block.TimeCapSec != nil {
		parts = append(parts, "лимит "+formatDurationSec(*block.TimeCapSec))
	}
	if work := workRestText(block.WorkSec, block.RestSec); work != "" {
		parts = append(parts, work)
	}
	return strings.Join(parts, " · ")
}

// workRestText renders the work and rest intervals as one part, for example
// "работа 20 с / отдых 10 с". A zero rest reads "без отдыха".
func workRestText(workSec, restSec *int) string {
	var parts []string
	if workSec != nil {
		parts = append(parts, "работа "+formatDurationSec(*workSec))
	}
	if restSec != nil {
		if *restSec == 0 {
			parts = append(parts, "без отдыха")
		} else {
			parts = append(parts, "отдых "+formatDurationSec(*restSec))
		}
	}
	return strings.Join(parts, " / ")
}

// roundsText returns n with the Russian plural form of "раунд".
func roundsText(n int) string {
	word := "раундов"
	if n%100 < 11 || n%100 > 14 {
		switch n % 10 {
		case 1:
			word = "раунд"
		case 2, 3, 4:
			word = "раунда"
		}
	}
	return fmt.Sprintf("%d %s", n, word)
}

// formatDurationSec renders seconds as "45 с", "12 мин", "1 мин 30 с" or
// "1 ч 1 мин 1 с", omitting zero components.
func formatDurationSec(sec int) string {
	hours, minutes, seconds := sec/3600, sec%3600/60, sec%60
	parts := make([]string, 0, 3)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d ч", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d мин", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d с", seconds))
	}
	return strings.Join(parts, " ")
}

func blockTypeIcon(t program.BlockType) string {
	switch t {
	case program.BlockTypeSuperset:
		return "🔗"
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
	case program.BlockTypeSkillWork:
		return "🎯"
	case program.BlockTypeStrength:
		return "🦾"
	case program.BlockTypeConditioning:
		return "🏃"
	case program.BlockTypeGymnastics:
		return "🤸"
	case program.BlockTypeWeightlifting:
		return "🏅"
	default:
		return ""
	}
}

func blockTypeLabel(t program.BlockType) string {
	switch t {
	case program.BlockTypeSuperset:
		return "Суперсет"
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
	case program.BlockTypeSkillWork:
		return "Skill Work"
	case program.BlockTypeStrength:
		return "Сила"
	case program.BlockTypeConditioning:
		return "Кондишн"
	case program.BlockTypeGymnastics:
		return "Гимнастика"
	case program.BlockTypeWeightlifting:
		return "Тяжёлая атлетика"
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
	switch {
	case errors.Is(err, trainerclient.ErrActiveTrainerNotSet):
		return "Выберите тренера в разделе «Тренеры»."
	case errors.Is(err, trainerclient.ErrTelegramUserNotFound):
		return "У вас пока нет тренеров. Откройте ссылку-приглашение от тренера."
	}
	return "Не удалось загрузить данные. Попробуйте позже."
}

func formatStatsMessage(trainerDisplayName string) string {
	return fmt.Sprintf("📊 Ваша статистика у тренера %s\n\nСсылка действует 30 минут.", trainerDisplayName)
}

func formatStatsUnavailable() string {
	return "Статистика пока недоступна."
}
