package telegrambot

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

func TestFormatToday_noProgram(t *testing.T) {
	text := formatToday(trainerclient.TelegramTodayResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         false,
	})
	if text == "" {
		t.Fatal("expected text")
	}
}

func TestFormatToday_restDay(t *testing.T) {
	text := formatToday(trainerclient.TelegramTodayResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		ProgramNameRu:      "Сила",
		WeekNumber:         1,
		DayNumber:          1,
		IsRestDay:          true,
	})
	if !strings.Contains(text, "😴 День отдыха") {
		t.Fatalf("expected rest day, got %q", text)
	}
	if !strings.Contains(text, "Неделя: 1 / День: 1") {
		t.Fatalf("expected week/day header, got %q", text)
	}
}

func TestFormatToday_header(t *testing.T) {
	text := formatToday(trainerclient.TelegramTodayResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		ProgramNameRu:      "Сила",
		WeekNumber:         2,
		DayNumber:          3,
		IsRestDay:          true,
	})
	want := []string{
		"📋 Сегодня",
		"👤 Тренер: Anna",
		"💪 Программа: Сила",
		"📆 Неделя: 2 / День: 3",
	}
	for _, part := range want {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in %q", part, text)
		}
	}
}

func TestFormatProgramSummary_noProgram(t *testing.T) {
	text := formatProgramSummary(trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         false,
	})
	if text == "" || !strings.Contains(text, "не назначил программу") {
		t.Fatalf("text = %q", text)
	}
}

func TestFormatProgramSummary_withCounts(t *testing.T) {
	sets, reps := "3", "10"
	text := formatProgramSummary(trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		Program: &program.Detail{
			Program: program.Program{NameRu: "Сила"},
			Weeks: []program.Week{
				{WeekNumber: 1, Days: []program.Day{{}}},
				{WeekNumber: 2, Days: []program.Day{{
					DayNumber: 1,
					Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{
						ExerciseNameRu: "Присед",
						Sets:           &sets,
						Reps:           &reps,
					}}}},
				}}},
			},
		},
	})
	for _, part := range []string{
		"👤 Тренер: Anna",
		"💪 Программа: Сила",
		"📆 Недель: 1",
		"🏋 Тренировочных дней: 1",
		"Выберите неделю",
	} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in %q", part, text)
		}
	}
}

func TestFormatProgramDay(t *testing.T) {
	sets, reps := "3", "10"
	text := formatProgramDay(2, 3, program.Day{
		Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{
			ExerciseNameRu: "Присед",
			Sets:           &sets,
			Reps:           &reps,
		}}}},
	})
	if !strings.Contains(text, "Неделя 2 / День 3") {
		t.Fatalf("text = %q", text)
	}
	if !strings.Contains(text, "🏋 Присед - 3х10") {
		t.Fatalf("text = %q", text)
	}
}

func TestFormatProgramWeekPicker(t *testing.T) {
	text := formatProgramWeekPicker(4)
	if text != "📆 Неделя 4\n\nВыберите день:" {
		t.Fatalf("text = %q", text)
	}
}

func TestFormatProgramEmptyTrainingDays(t *testing.T) {
	text := formatProgramEmptyTrainingDays("Anna")
	if !strings.Contains(text, "Anna") || !strings.Contains(text, "тренировочные дни") {
		t.Fatalf("text = %q", text)
	}
}

func TestFormatProgramNotFoundMessage(t *testing.T) {
	if formatProgramNotFoundMessage() == "" {
		t.Fatal("expected message")
	}
}

func TestFormatTrainers_empty(t *testing.T) {
	text, trainers := formatTrainers(trainerclient.TelegramTrainerList{})
	if !strings.Contains(text, "👤 Тренеры") || trainers != nil {
		t.Fatalf("text=%q trainers=%v", text, trainers)
	}
}

func TestFormatTrainers_single(t *testing.T) {
	text, trainers := formatTrainers(trainerclient.TelegramTrainerList{
		Items: []trainerclient.TelegramTrainer{{
			DisplayName:   "Anna",
			HasProgram:    true,
			ProgramNameRu: "Сила",
		}},
	})
	if trainers != nil {
		t.Fatalf("trainers=%v", trainers)
	}
	for _, part := range []string{"👤 Тренеры", "👤 Тренер: Anna", "💪 Программа: Сила"} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in %q", part, text)
		}
	}
}

func TestFormatTrainers_singleNoProgram(t *testing.T) {
	text, _ := formatTrainers(trainerclient.TelegramTrainerList{
		Items: []trainerclient.TelegramTrainer{{DisplayName: "Anna"}},
	})
	if !strings.Contains(text, "💪 Программа не назначена") {
		t.Fatalf("text=%q", text)
	}
}

func TestFormatBlocks_groupExerciseInstruction(t *testing.T) {
	sets, reps := "3", "5"
	text := formatBlocks([]program.DayBlock{{
		BlockType: program.BlockTypeComplex,
		Exercises: []program.DayExercise{{
			ExerciseNameRu: "Подтягивания",
			Sets:           &sets,
			Reps:           &reps,
			Instruction:    "полная амплитуда",
		}},
	}})
	if !strings.Contains(text, "полная амплитуда") {
		t.Fatalf("expected exercise instruction, got %q", text)
	}
}

func TestProgramWeeksKeyboard_filtersEmptyWeeks(t *testing.T) {
	sets, reps := "3", "10"
	kb := programWeeksKeyboard([]program.Week{
		{WeekNumber: 1, Days: []program.Day{{}}},
		{WeekNumber: 2, Days: []program.Day{{
			Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{Sets: &sets, Reps: &reps}}}},
		}}},
	}, nil)
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("rows = %d, want 1", len(kb.InlineKeyboard))
	}
	if kb.InlineKeyboard[0][0].Text != "Неделя 2" {
		t.Fatalf("label = %q", kb.InlineKeyboard[0][0].Text)
	}
}

func TestProgramWeeksKeyboard_marksCompletedWeek(t *testing.T) {
	sets, reps := "3", "10"
	ex := program.DayExercise{Sets: &sets, Reps: &reps}
	d1, d2, d3 := uuid.New(), uuid.New(), uuid.New()
	weeks := []program.Week{
		{
			WeekNumber: 1,
			Days: []program.Day{
				{DayNumber: 1, DayKey: d1, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
				{DayNumber: 2, DayKey: d2, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
			},
		},
		{
			WeekNumber: 2,
			Days: []program.Day{
				{DayNumber: 1, DayKey: d3, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
			},
		},
	}
	kb := programWeeksKeyboard(weeks, map[uuid.UUID]struct{}{d1: {}, d2: {}})
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("rows = %d, want 2", len(kb.InlineKeyboard))
	}
	if kb.InlineKeyboard[0][0].Text != "✅ Неделя 1" {
		t.Fatalf("week1 = %q", kb.InlineKeyboard[0][0].Text)
	}
	if kb.InlineKeyboard[1][0].Text != "Неделя 2" {
		t.Fatalf("week2 = %q", kb.InlineKeyboard[1][0].Text)
	}
}

func TestProgramDaysKeyboard_filtersEmptyDays(t *testing.T) {
	sets, reps := "3", "10"
	kb := programDaysKeyboard(1, []program.Day{
		{DayNumber: 1},
		{DayNumber: 2, Blocks: []program.DayBlock{{BlockType: program.BlockTypeComplex}}},
		{DayNumber: 3, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{{Sets: &sets, Reps: &reps}}}}},
	}, nil)
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("rows = %d, want 1", len(kb.InlineKeyboard))
	}
	if kb.InlineKeyboard[0][0].Text != "День 3" {
		t.Fatalf("label = %q", kb.InlineKeyboard[0][0].Text)
	}
}

func TestProgramDaysKeyboard_marksCompleted(t *testing.T) {
	sets, reps := "3", "10"
	done := uuid.New()
	open := uuid.New()
	ex := program.DayExercise{Sets: &sets, Reps: &reps}
	kb := programDaysKeyboard(1, []program.Day{
		{DayNumber: 1, DayKey: done, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
		{DayNumber: 2, DayKey: open, Blocks: []program.DayBlock{{Exercises: []program.DayExercise{ex}}}},
	}, map[uuid.UUID]struct{}{done: {}})
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("rows = %d, want 2", len(kb.InlineKeyboard))
	}
	if kb.InlineKeyboard[0][0].Text != "✅ День 1" {
		t.Fatalf("day1 = %q", kb.InlineKeyboard[0][0].Text)
	}
	if kb.InlineKeyboard[1][0].Text != "День 2" {
		t.Fatalf("day2 = %q", kb.InlineKeyboard[1][0].Text)
	}
}

func TestParseProgramCallbacks(t *testing.T) {
	week, ok := parseProgramWeekCallback(programWeekCallbackData(2))
	if !ok || week != 2 {
		t.Fatalf("week = %d ok=%v", week, ok)
	}
	gotWeek, gotDay, ok := parseProgramDayCallback(programDayCallbackData(2, 3))
	if !ok || gotWeek != 2 || gotDay != 3 {
		t.Fatalf("week=%d day=%d ok=%v", gotWeek, gotDay, ok)
	}
	if _, ok := parseProgramWeekCallback("bad"); ok {
		t.Fatal("expected invalid week callback")
	}
	if _, _, ok := parseProgramDayCallback("bad"); ok {
		t.Fatal("expected invalid day callback")
	}
}

func TestProgramDayNavKeyboard_nextDayAndWeek(t *testing.T) {
	sets, reps := "3", "10"
	ex := []program.DayBlock{{Exercises: []program.DayExercise{{Sets: &sets, Reps: &reps}}}}
	weeks := []program.Week{
		{WeekNumber: 1, Days: []program.Day{
			{DayNumber: 1, Blocks: ex},
			{DayNumber: 2, Blocks: ex},
		}},
		{WeekNumber: 2, Days: []program.Day{{DayNumber: 1, Blocks: ex}}},
	}

	kb := programDayNavKeyboard(weeks, 1, 1)
	if len(kb.InlineKeyboard) != 1 || kb.InlineKeyboard[0][0].Text != "Следующий день" {
		t.Fatalf("day 1 nav = %+v", kb.InlineKeyboard)
	}

	kb = programDayNavKeyboard(weeks, 1, 2)
	if len(kb.InlineKeyboard) != 1 || kb.InlineKeyboard[0][0].Text != "Следующая неделя" {
		t.Fatalf("last day nav = %+v", kb.InlineKeyboard)
	}

	kb = programDayNavKeyboard(weeks, 2, 1)
	if len(kb.InlineKeyboard) != 0 {
		t.Fatalf("final day nav = %+v", kb.InlineKeyboard)
	}
}

func TestFormatBlocks_withInstruction(t *testing.T) {
	sets, reps := "3", "5"
	text := formatBlocks([]program.DayBlock{{
		BlockType:   program.BlockTypeComplex,
		Instruction: "3 раунда, отдых 2 мин",
		Exercises: []program.DayExercise{
			{ExerciseNameRu: "Подтягивания", Sets: &sets, Reps: &reps},
			{ExerciseNameRu: "Жим", Sets: &sets, Reps: &reps},
		},
	}})
	if !strings.Contains(text, "🧩 Комплекс:") {
		t.Fatalf("expected complex header, got %q", text)
	}
	if !strings.Contains(text, "3 раунда") {
		t.Fatalf("expected block instruction, got %q", text)
	}
	if !strings.Contains(text, "А. Подтягивания - 3х5") {
		t.Fatalf("expected exercise A, got %q", text)
	}
	if !strings.Contains(text, "Б. Жим - 3х5") {
		t.Fatalf("expected exercise B, got %q", text)
	}
}

func TestFormatBlocks_exerciseInstruction(t *testing.T) {
	sets, reps := "3", "10"
	text := formatBlocks([]program.DayBlock{{
		Exercises: []program.DayExercise{{
			ExerciseNameRu: "Присед",
			Sets:           &sets,
			Reps:           &reps,
			Instruction:    "медленно вниз",
		}},
	}})
	if !strings.Contains(text, "🏋 Присед - 3х10") {
		t.Fatalf("expected volume, got %q", text)
	}
	if !strings.Contains(text, "медленно вниз") {
		t.Fatalf("expected exercise instruction, got %q", text)
	}
}

func TestFormatBlocks_multipleStandaloneExercises(t *testing.T) {
	sets, reps := "3", "8"
	text := formatBlocks([]program.DayBlock{
		{Exercises: []program.DayExercise{{
			ExerciseNameRu: "Бросок мяча",
			Sets:           &sets,
			Reps:           &reps,
		}}},
		{Exercises: []program.DayExercise{{
			ExerciseNameRu: "Подтягивания",
			Sets:           &sets,
			Reps:           &reps,
		}}},
	})
	if !strings.Contains(text, "🏋 Бросок мяча - 3х8") {
		t.Fatalf("expected first exercise, got %q", text)
	}
	if !strings.Contains(text, "🏋 Подтягивания - 3х8") {
		t.Fatalf("expected second exercise, got %q", text)
	}
	if !strings.Contains(text, "\n\n") {
		t.Fatalf("expected blank line between exercises, got %q", text)
	}
}

func TestFormatBlocks(t *testing.T) {
	sets, reps := "3", "10"
	text := formatBlocks([]program.DayBlock{{
		Exercises: []program.DayExercise{{
			ExerciseNameRu: "Присед",
			Sets:           &sets,
			Reps:           &reps,
		}},
	}})
	if text == "" {
		t.Fatal("expected formatted blocks")
	}
}

func TestFormatToday_withBlocks(t *testing.T) {
	sets, reps := "3", "10"
	text := formatToday(trainerclient.TelegramTodayResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		ProgramNameRu:      "Сила",
		ProgramDayNumber:   2,
		WeekNumber:         1,
		DayNumber:          2,
		Blocks: []program.DayBlock{{
			Exercises: []program.DayExercise{{
				ExerciseNameRu: "Присед",
				Sets:           &sets,
				Reps:           &reps,
			}},
		}},
	})
	if text == "" {
		t.Fatal("expected text")
	}
}

func TestFormatActiveTrainerNotification(t *testing.T) {
	text := formatActiveTrainerNotification(trainerclient.TelegramTrainer{
		DisplayName:   "Anna",
		HasProgram:    true,
		ProgramNameRu: "Сила",
	})
	for _, part := range []string{"✓ 👤 Тренер: Anna", "💪 Программа: Сила"} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in %q", part, text)
		}
	}
}

func TestFormatTrainers_multi(t *testing.T) {
	text, trainers := formatTrainers(trainerclient.TelegramTrainerList{
		Items: []trainerclient.TelegramTrainer{
			{TrainerID: uuid.New(), DisplayName: "Anna", IsActive: true, HasProgram: true, ProgramNameRu: "Сила"},
			{TrainerID: uuid.New(), DisplayName: "Ivan"},
		},
	})
	if len(trainers) != 2 {
		t.Fatalf("trainers=%d", len(trainers))
	}
	for _, part := range []string{
		"👤 Тренеры",
		"✓ 👤 Тренер: Anna",
		"💪 Программа: Сила",
		"👤 Тренер: Ivan",
		"💪 Программа не назначена",
	} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in %q", part, text)
		}
	}
}

func TestMenuErrorText_conflict(t *testing.T) {
	if menuErrorText(trainerclient.ErrActiveTrainerNotSet) == "" {
		t.Fatal("expected text")
	}
}

func TestMenuErrorText_default(t *testing.T) {
	if menuErrorText(http.ErrServerClosed) == "" {
		t.Fatal("expected text")
	}
}

func TestProgramDisplayName_english(t *testing.T) {
	if programDisplayName("Eng", "") != "Eng" {
		t.Fatal("expected eng")
	}
}

func TestEscapeTelegramMarkdown(t *testing.T) {
	got := escapeTelegramMarkdown("a_b*c")
	if got != `a\_b\*c` {
		t.Fatalf("got %q", got)
	}
}

func TestParseTrainerCallback(t *testing.T) {
	id := uuid.New()
	parsed, ok := parseTrainerCallback(trainerCallbackData(id))
	if !ok || parsed != id {
		t.Fatalf("parsed=%v ok=%v", parsed, ok)
	}
}
