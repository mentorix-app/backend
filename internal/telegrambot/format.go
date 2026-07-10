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
	fmt.Fprintf(&b, "📋 *Сегодня*\n👤 %s · 💪 %s\n📆 День %d · Н%d Д%d\n",
		escapeTelegramMarkdown(resp.TrainerDisplayName),
		escapeTelegramMarkdown(name),
		resp.ProgramDayNumber, resp.WeekNumber, resp.DayNumber)
	if resp.IsRestDay {
		b.WriteString("\n😴 *День отдыха*")
		return b.String()
	}
	if blockText := formatBlocks(resp.Blocks); blockText != "" {
		b.WriteString("\n\n")
		b.WriteString(blockText)
	}
	return b.String()
}

func formatProgram(resp trainerclient.TelegramProgramResponse) []string {
	if !resp.HasProgram || resp.Program == nil {
		return []string{fmt.Sprintf("%s пока не назначил программу.", resp.TrainerDisplayName)}
	}
	name := programDisplayName(resp.Program.Name, resp.Program.NameRu)
	header := fmt.Sprintf("📅 *Программа*\n👤 %s · 💪 %s\n",
		escapeTelegramMarkdown(resp.TrainerDisplayName),
		escapeTelegramMarkdown(name))

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
			dayHeader := fmt.Sprintf("📆 *День %d*\n", day.DayNumber)
			if current.Len()+len(dayHeader) > telegramMessageLimit {
				parts = append(parts, current.String())
				current.Reset()
			}
			current.WriteString(dayHeader)
			blockText := formatBlocks(day.Blocks)
			if blockText == "" {
				blockText = "😴 отдых\n"
			}
			if current.Len()+len(blockText) > telegramMessageLimit {
				parts = append(parts, current.String())
				current.Reset()
			}
			current.WriteString(blockText)
			current.WriteString("\n")
		}
	}
	if current.Len() > 0 {
		parts = append(parts, strings.TrimRight(current.String(), "\n"))
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
		fmt.Fprintf(&b, "👤 *%s*\n", escapeTelegramMarkdown(t.DisplayName))
		if t.HasProgram {
			b.WriteString("💪 " + escapeTelegramMarkdown(programDisplayName(t.ProgramName, t.ProgramNameRu)))
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
		line := fmt.Sprintf("%s%s", mark, escapeTelegramMarkdown(t.DisplayName))
		if t.HasProgram {
			line += " — " + escapeTelegramMarkdown(programDisplayName(t.ProgramName, t.ProgramNameRu))
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

	icon, label := blockTypeDisplay(block.BlockType)
	var b strings.Builder

	if blockIsGroup(block.BlockType) {
		writeBlockHeader(&b, icon, label, block.Instruction)
		for i, ex := range block.Exercises {
			if i > 0 {
				b.WriteString("\n")
			}
			writeExercise(&b, ex, "  "+exerciseLetter(i)+". ")
		}
		return b.String()
	}

	for i, ex := range block.Exercises {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s ", icon)
		writeExercise(&b, ex, "")
	}
	if instr := strings.TrimSpace(block.Instruction); instr != "" {
		b.WriteString("\n")
		writeInstruction(&b, "   ", instr)
	}
	return b.String()
}

func writeBlockHeader(b *strings.Builder, icon, label, instruction string) {
	fmt.Fprintf(b, "%s *%s*", icon, label)
	if instr := strings.TrimSpace(instruction); instr != "" {
		b.WriteString("\n")
		writeInstruction(b, "   ", instr)
	} else {
		b.WriteString("\n")
	}
}

func writeExercise(b *strings.Builder, ex program.DayExercise, prefix string) {
	name := exerciseDisplayName(ex.ExerciseName, ex.ExerciseNameRu)
	line := prefix + escapeTelegramMarkdown(name)
	if vol := formatExerciseVolume(ex); vol != "" {
		line += " — " + vol
	}
	b.WriteString(line)
	if instr := strings.TrimSpace(ex.Instruction); instr != "" {
		b.WriteString("\n")
		writeInstruction(b, "     ", instr)
	}
}

func writeInstruction(b *strings.Builder, indent, instruction string) {
	fmt.Fprintf(b, "%s_%s_", indent, escapeTelegramMarkdown(instruction))
}

func formatExerciseVolume(ex program.DayExercise) string {
	switch {
	case ex.Sets != nil && ex.Reps != nil:
		return fmt.Sprintf("%d×%d", *ex.Sets, *ex.Reps)
	case ex.Sets != nil:
		return fmt.Sprintf("%d подх.", *ex.Sets)
	case ex.Reps != nil:
		return fmt.Sprintf("%d повт.", *ex.Reps)
	default:
		return ""
	}
}

func exerciseLetter(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return fmt.Sprintf("%d", i+1)
}

func blockIsGroup(t program.BlockType) bool {
	switch t {
	case program.BlockTypeEMOM, program.BlockTypeAMRAP, program.BlockTypeForTime,
		program.BlockTypeIntervals, program.BlockTypeChipper, program.BlockTypeLadder,
		program.BlockTypeDeathBy, program.BlockTypeSuperset, program.BlockTypeComplex:
		return true
	default:
		return false
	}
}

func blockTypeDisplay(t program.BlockType) (icon, label string) {
	switch t {
	case program.BlockTypeSuperset:
		return "🔗", "Суперсет"
	case program.BlockTypeComplex:
		return "🧩", "Комплекс"
	case program.BlockTypeEMOM:
		return "⏱", "EMOM"
	case program.BlockTypeAMRAP:
		return "🔁", "AMRAP"
	case program.BlockTypeForTime:
		return "🏁", "For Time"
	case program.BlockTypeIntervals:
		return "⚡", "Интервалы"
	case program.BlockTypeChipper:
		return "📉", "Чиппер"
	case program.BlockTypeLadder:
		return "📈", "Лестница"
	case program.BlockTypeDeathBy:
		return "💀", "Death By"
	default:
		return "▫️", ""
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
