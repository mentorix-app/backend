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

func TestFormatProgram_noProgram(t *testing.T) {
	parts := formatProgram(trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         false,
	})
	if len(parts) != 1 {
		t.Fatalf("parts = %d", len(parts))
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
	sets, reps := 3, 5
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

func TestFormatProgram_splitsLongMessage(t *testing.T) {
	sets, reps := 3, 10
	ex := program.DayExercise{
		ExerciseNameRu: "Присед",
		Sets:           &sets,
		Reps:           &reps,
		Instruction:    strings.Repeat("объём ", 500),
	}
	days := make([]program.Day, 5)
	for i := range days {
		days[i] = program.Day{
			DayNumber: i + 1,
			Blocks:    []program.DayBlock{{Exercises: []program.DayExercise{ex}}},
		}
	}
	parts := formatProgram(trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		Program: &program.Detail{
			Program: program.Program{NameRu: "Сила"},
			Weeks: []program.Week{{
				WeekNumber: 1,
				Days:       days,
			}},
		},
	})
	if len(parts) < 2 {
		t.Fatalf("expected message split, got %d parts", len(parts))
	}
}

func TestFormatBlocks_withInstruction(t *testing.T) {
	sets, reps := 3, 5
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
	sets, reps := 3, 10
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
	sets, reps := 3, 8
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
	sets, reps := 3, 10
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
	sets, reps := 3, 10
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

func TestFormatProgram_withWeeks(t *testing.T) {
	sets, reps := 3, 10
	parts := formatProgram(trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		Program: &program.Detail{
			Program: program.Program{NameRu: "Сила"},
			Weeks: []program.Week{{
				WeekNumber: 1,
				Days: []program.Day{{
					DayNumber: 1,
					Blocks: []program.DayBlock{{
						Exercises: []program.DayExercise{{
							ExerciseNameRu: "Присед",
							Sets:           &sets,
							Reps:           &reps,
						}},
					}},
				}},
			}},
		},
	})
	if len(parts) == 0 || parts[0] == "" {
		t.Fatal("expected program text")
	}
}

func TestFormatProgram_headerAndRestDay(t *testing.T) {
	parts := formatProgram(trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		Program: &program.Detail{
			Program: program.Program{NameRu: "Сила"},
			Weeks: []program.Week{{
				WeekNumber: 1,
				Days:       []program.Day{{DayNumber: 1, Blocks: nil}},
			}},
		},
	})
	if len(parts) == 0 || !strings.Contains(parts[0], "📅 Программа") {
		t.Fatalf("parts = %+v", parts)
	}
	if !strings.Contains(parts[0], "👤 Тренер: Anna") {
		t.Fatalf("parts = %+v", parts)
	}
	if !strings.Contains(parts[0], "💪 Программа: Сила") {
		t.Fatalf("parts = %+v", parts)
	}
	if !strings.Contains(parts[0], "📆 Неделя 1") {
		t.Fatalf("parts = %+v", parts)
	}
	if !strings.Contains(parts[0], "📆 День 1") {
		t.Fatalf("parts = %+v", parts)
	}
	if !strings.Contains(parts[0], "😴 отдых") {
		t.Fatalf("parts = %+v", parts)
	}
}

func TestFormatProgram_restDayInWeek(t *testing.T) {
	parts := formatProgram(trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		Program: &program.Detail{
			Program: program.Program{NameRu: "Сила"},
			Weeks: []program.Week{{
				WeekNumber: 1,
				Days:       []program.Day{{DayNumber: 1, Blocks: nil}},
			}},
		},
	})
	if len(parts) == 0 || !strings.Contains(parts[0], "отдых") {
		t.Fatalf("parts = %+v", parts)
	}
}

func TestFormatProgram_nilProgramUsesEmptyMessage(t *testing.T) {
	parts := formatProgram(trainerclient.TelegramProgramResponse{
		TrainerDisplayName: "Anna",
		HasProgram:         true,
		Program:            nil,
	})
	if len(parts) != 1 {
		t.Fatalf("parts = %d", len(parts))
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
