package telegramnotify

import (
	"strings"
	"testing"
)

func TestAssignedMessage(t *testing.T) {
	got := assignedMessage("Иван", "Сила")
	want := "Тренер Иван назначил вам программу «Сила».\n\nОткройте «Программа» в меню."
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestSyncedMessage(t *testing.T) {
	got := syncedMessage("Иван", "Сила")
	want := "Тренер Иван обновил вашу программу «Сила».\n\nОткройте «Программа» в меню, чтобы посмотреть изменения."
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestWorkoutCommentMessage(t *testing.T) {
	got := workoutCommentMessage("Иван", WorkoutComment{
		ProgramName:   "Strength",
		ProgramNameRu: "Сила",
		WeekNumber:    2,
		DayNumber:     3,
		ResultText:    "присед 5х5 90 кг",
		CommentText:   "Отличная работа!",
	})
	want := "Тренер Иван ответил на ваш результат тренировки «Сила», неделя 2, день 3.\n\n" +
		"Ваш результат:\nприсед 5х5 90 кг\n\n" +
		"Ответ тренера:\nОтличная работа!"
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestWorkoutCommentMessage_truncatesLongResult(t *testing.T) {
	long := strings.Repeat("ы", maxResultQuoteLen+100)
	got := workoutCommentMessage("Иван", WorkoutComment{
		ProgramName: "Strength",
		WeekNumber:  1,
		DayNumber:   1,
		ResultText:  long,
		CommentText: "ok",
	})
	wantQuote := strings.Repeat("ы", maxResultQuoteLen) + "…"
	if !strings.Contains(got, wantQuote) {
		t.Fatalf("message does not contain truncated quote")
	}
	if strings.Contains(got, long) {
		t.Fatalf("message contains full result text")
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"абвгд", 5, "абвгд"},
		{"абвгде", 5, "абвгд…"},
		{"a b ", 3, "a b…"},
	}
	for _, tt := range tests {
		if got := truncateRunes(tt.in, tt.max); got != tt.want {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
		}
	}
}

func TestProgramDisplayName_prefersRu(t *testing.T) {
	got := programDisplayName("Strength", "Сила")
	if got != "Сила" {
		t.Fatalf("name = %q, want Сила", got)
	}
}

func TestProgramDisplayName_fallsBackToName(t *testing.T) {
	got := programDisplayName("Strength", "")
	if got != "Strength" {
		t.Fatalf("name = %q, want Strength", got)
	}
}
