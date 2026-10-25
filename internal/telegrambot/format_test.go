package telegrambot

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

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
		BlockType: program.BlockTypeSkillWork,
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
		{DayNumber: 2, Blocks: []program.DayBlock{{BlockType: program.BlockTypeSkillWork}}},
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
		BlockType:   program.BlockTypeSkillWork,
		Instruction: "3 раунда, отдых 2 мин",
		Exercises: []program.DayExercise{
			{ExerciseNameRu: "Подтягивания", Sets: &sets, Reps: &reps},
			{ExerciseNameRu: "Жим", Sets: &sets, Reps: &reps},
		},
	}})
	if !strings.Contains(text, "🎯 Skill Work:") {
		t.Fatalf("expected skill work header, got %q", text)
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

func TestBlockTypeHeading_newTypes(t *testing.T) {
	tests := []struct {
		t    program.BlockType
		want string
	}{
		{program.BlockTypeSkillWork, "🎯 Skill Work"},
		{program.BlockTypeStrength, "🦾 Сила"},
		{program.BlockTypeConditioning, "🏃 Кондишн"},
		{program.BlockTypeGymnastics, "🤸 Гимнастика"},
		{program.BlockTypeWeightlifting, "🏅 Тяжёлая атлетика"},
	}
	for _, tt := range tests {
		if got := blockTypeHeading(tt.t); got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.t, got, tt.want)
		}
	}
}

func TestBlockTypeIcon_uniqueAmongGroupTypes(t *testing.T) {
	groupTypes := []program.BlockType{
		program.BlockTypeEMOM, program.BlockTypeAMRAP, program.BlockTypeForTime,
		program.BlockTypeIntervals, program.BlockTypeChipper, program.BlockTypeSuperset,
		program.BlockTypeSkillWork, program.BlockTypeStrength, program.BlockTypeConditioning,
		program.BlockTypeGymnastics, program.BlockTypeWeightlifting,
	}
	seen := make(map[string]program.BlockType, len(groupTypes))
	for _, bt := range groupTypes {
		icon := blockTypeIcon(bt)
		if icon == "" {
			t.Fatalf("%s: empty icon", bt)
		}
		if icon == "🏋" {
			t.Fatalf("%s: icon %q collides with standalone exercise bullet", bt, icon)
		}
		if prev, ok := seen[icon]; ok {
			t.Fatalf("icon %q used by both %s and %s", icon, prev, bt)
		}
		seen[icon] = bt
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

func TestFormatBlock_settingsLine(t *testing.T) {
	n := func(v int) *int { return &v }
	tests := []struct {
		name  string
		block program.DayBlock
		want  string
	}{
		{"emom rounds", program.DayBlock{BlockType: program.BlockTypeEMOM, Rounds: n(10)}, "⏱ EMOM:\n*10 раундов*\n\n"},
		{"amrap duration", program.DayBlock{BlockType: program.BlockTypeAMRAP, DurationSec: n(720)}, "🔁 AMRAP:\n*12 мин*\n\n"},
		{"for time rounds and cap", program.DayBlock{BlockType: program.BlockTypeForTime, Rounds: n(5), TimeCapSec: n(900)}, "🏁 For Time:\n*5 раундов · лимит 15 мин*\n\n"},
		{"for time only rounds", program.DayBlock{BlockType: program.BlockTypeForTime, Rounds: n(5)}, "🏁 For Time:\n*5 раундов*\n\n"},
		{"for time only cap", program.DayBlock{BlockType: program.BlockTypeForTime, TimeCapSec: n(900)}, "🏁 For Time:\n*лимит 15 мин*\n\n"},
		{"intervals work and rest", program.DayBlock{BlockType: program.BlockTypeIntervals, Rounds: n(8), WorkSec: n(20), RestSec: n(10)}, "⚡ Интервалы:\n*8 раундов · работа 20 с / отдых 10 с*\n\n"},
		{"intervals zero rest", program.DayBlock{BlockType: program.BlockTypeIntervals, Rounds: n(8), WorkSec: n(20), RestSec: n(0)}, "⚡ Интервалы:\n*8 раундов · работа 20 с / без отдыха*\n\n"},
		{"intervals only work", program.DayBlock{BlockType: program.BlockTypeIntervals, WorkSec: n(20)}, "⚡ Интервалы:\n*работа 20 с*\n\n"},
		{"intervals only rest", program.DayBlock{BlockType: program.BlockTypeIntervals, RestSec: n(10)}, "⚡ Интервалы:\n*отдых 10 с*\n\n"},
		{"intervals only zero rest", program.DayBlock{BlockType: program.BlockTypeIntervals, RestSec: n(0)}, "⚡ Интервалы:\n*без отдыха*\n\n"},
		{"superset rounds", program.DayBlock{BlockType: program.BlockTypeSuperset, Rounds: n(3)}, "🔗 Суперсет:\n*3 раунда*\n\n"},
		{"mixed duration", program.DayBlock{BlockType: program.BlockTypeAMRAP, DurationSec: n(90)}, "🔁 AMRAP:\n*1 мин 30 с*\n\n"},
		{"all fields in order", program.DayBlock{BlockType: program.BlockTypeIntervals, Rounds: n(2), DurationSec: n(600), TimeCapSec: n(60), WorkSec: n(30), RestSec: n(15)},
			"⚡ Интервалы:\n*2 раунда · 10 мин · лимит 1 мин · работа 30 с / отдых 15 с*\n\n"},
		{"settings and instruction", program.DayBlock{BlockType: program.BlockTypeEMOM, Rounds: n(10), Instruction: "темп ровный"}, "⏱ EMOM:\n*10 раундов*\n_темп ровный_\n\n"},
		{"instruction without settings", program.DayBlock{BlockType: program.BlockTypeEMOM, Instruction: "темп ровный"}, "⏱ EMOM:\n_темп ровный_\n\n"},
		{"no settings", program.DayBlock{BlockType: program.BlockTypeEMOM}, "⏱ EMOM:\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.block.Exercises = []program.DayExercise{{ExerciseNameRu: "Присед"}}
			text := formatBlock(tt.block)
			head, tail, ok := strings.Cut(text, "А. Присед")
			if !ok {
				t.Fatalf("exercise line missing in %q", text)
			}
			if tail != "" {
				t.Fatalf("unexpected text after the exercise in %q", text)
			}
			if got, want := head, tt.want; got != want {
				t.Fatalf("lines before the exercise = %q, want %q", got, want)
			}
		})
	}
}

func TestFormatBlock_fullLayout(t *testing.T) {
	rounds, work, rest := 8, 20, 10
	reps := "10"
	text := formatBlock(program.DayBlock{
		BlockType:   program.BlockTypeIntervals,
		Rounds:      &rounds,
		WorkSec:     &work,
		RestSec:     &rest,
		Instruction: "без пауз",
		Exercises:   []program.DayExercise{{ExerciseNameRu: "Берпи", Reps: &reps}},
	})
	want := "⚡ Интервалы:\n*8 раундов · работа 20 с / отдых 10 с*\n_без пауз_\n\nА. Берпи - 10 повт."
	if text != want {
		t.Fatalf("block = %q, want %q", text, want)
	}
}

func TestFormatBlock_noSettingsLayout(t *testing.T) {
	reps := "10"
	block := program.DayBlock{
		BlockType: program.BlockTypeEMOM,
		Exercises: []program.DayExercise{{ExerciseNameRu: "Берпи", Reps: &reps}},
	}
	if got, want := formatBlock(block), "⏱ EMOM:\n\nА. Берпи - 10 повт."; got != want {
		t.Fatalf("block = %q, want %q", got, want)
	}
	block.Instruction = "темп ровный"
	if got, want := formatBlock(block), "⏱ EMOM:\n_темп ровный_\n\nА. Берпи - 10 повт."; got != want {
		t.Fatalf("block with instruction = %q, want %q", got, want)
	}
}

func TestFormatBlock_singleBlockIgnoresSettings(t *testing.T) {
	rounds := 3
	text := formatBlock(program.DayBlock{
		BlockType: program.BlockTypeSingle,
		Rounds:    &rounds,
		Exercises: []program.DayExercise{{ExerciseNameRu: "Присед"}},
	})
	if text != "🏋 Присед" {
		t.Fatalf("single block = %q", text)
	}
}

func TestRoundsText(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{1, "1 раунд"}, {2, "2 раунда"}, {4, "4 раунда"}, {5, "5 раундов"},
		{11, "11 раундов"}, {12, "12 раундов"}, {14, "14 раундов"}, {20, "20 раундов"},
		{21, "21 раунд"}, {22, "22 раунда"}, {25, "25 раундов"}, {100, "100 раундов"},
		{101, "101 раунд"}, {111, "111 раундов"}, {13, "13 раундов"}, {113, "113 раундов"},
	}
	for _, tt := range tests {
		if got := roundsText(tt.n); got != tt.want {
			t.Errorf("roundsText(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatDurationSec(t *testing.T) {
	tests := []struct {
		sec  int
		want string
	}{
		{1, "1 с"}, {45, "45 с"}, {59, "59 с"}, {60, "1 мин"}, {90, "1 мин 30 с"},
		{720, "12 мин"}, {3599, "59 мин 59 с"}, {3600, "1 ч"}, {3660, "1 ч 1 мин"},
		{3661, "1 ч 1 мин 1 с"}, {3601, "1 ч 1 с"}, {5400, "1 ч 30 мин"}, {86400, "24 ч"},
	}
	for _, tt := range tests {
		if got := formatDurationSec(tt.sec); got != tt.want {
			t.Errorf("formatDurationSec(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}
