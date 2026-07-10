package telegramnotify

import "testing"

func TestAssignedMessage(t *testing.T) {
	got := assignedMessage("Иван", "Сила")
	want := "Тренер Иван назначил вам программу «Сила».\n\nОткройте «Сегодня» или «Программа» в меню."
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
