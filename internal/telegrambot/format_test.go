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
		ProgramDayNumber:   1,
		WeekNumber:         1,
		DayNumber:          1,
		IsRestDay:          true,
	})
	if !strings.Contains(text, "отдыха") {
		t.Fatalf("expected rest day, got %q", text)
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
	if text == "" || trainers != nil {
		t.Fatalf("text=%q trainers=%v", text, trainers)
	}
}

func TestFormatTrainers_single(t *testing.T) {
	text, trainers := formatTrainers(trainerclient.TelegramTrainerList{
		Items: []trainerclient.TelegramTrainer{{DisplayName: "Anna"}},
	})
	if text == "" || trainers != nil {
		t.Fatalf("text=%q trainers=%v", text, trainers)
	}
}

func TestFormatBlocks_withInstruction(t *testing.T) {
	text := formatBlocks([]program.DayBlock{{
		BlockType:   program.BlockTypeSuperset,
		Instruction: "3 раунда, отдых 2 мин",
		Exercises:   []program.DayExercise{{ExerciseNameRu: "Бег"}},
	}})
	if !strings.Contains(text, "🔗") || !strings.Contains(text, "Суперсет") {
		t.Fatalf("expected superset header, got %q", text)
	}
	if !strings.Contains(text, "3 раунда") {
		t.Fatalf("expected block instruction, got %q", text)
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
	if !strings.Contains(text, "3×10") {
		t.Fatalf("expected volume, got %q", text)
	}
	if !strings.Contains(text, "медленно вниз") {
		t.Fatalf("expected exercise instruction, got %q", text)
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
			{TrainerID: uuid.New(), DisplayName: "Anna", IsActive: true},
			{TrainerID: uuid.New(), DisplayName: "Ivan"},
		},
	})
	if text == "" || len(trainers) != 2 {
		t.Fatalf("text=%q trainers=%d", text, len(trainers))
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
