package trainerclient

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestHTTPErrorFrom_activeTrainerNotSet(t *testing.T) {
	err := HTTPErrorFrom(ErrActiveTrainerNotSet)
	if err == nil || err.Code != 409 {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPErrorFrom_telegramUserNotFound(t *testing.T) {
	err := HTTPErrorFrom(ErrTelegramUserNotFound)
	if err == nil || err.Code != 404 {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPErrorFrom_trainerNotLinked(t *testing.T) {
	err := HTTPErrorFrom(ErrTrainerNotLinked)
	if err == nil || err.Code != 403 {
		t.Fatalf("err = %v", err)
	}
}

func TestPickProgramName_prefersRu(t *testing.T) {
	if got := pickProgramName("Eng", "Рус"); got != "Рус" {
		t.Fatalf("got = %q", got)
	}
}

func TestResolveActiveTrainerID_singleTrainer(t *testing.T) {
	svc := &Service{activeTrainer: NewMemoryActiveTrainerStore()}
	id := uuid.New()
	got, err := svc.resolveActiveTrainerID(t.Context(), "1", []TelegramTrainer{{TrainerID: id}})
	if err != nil || got == nil || *got != id {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestResolveActiveTrainerID_fromStore(t *testing.T) {
	store := NewMemoryActiveTrainerStore()
	id := uuid.New()
	_ = store.Set(t.Context(), "1", id)
	svc := &Service{activeTrainer: store}
	got, err := svc.resolveActiveTrainerID(t.Context(), "1", []TelegramTrainer{
		{TrainerID: id},
		{TrainerID: uuid.New()},
	})
	if err != nil || got == nil || *got != id {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestResolveActiveTrainerID_notSet(t *testing.T) {
	svc := &Service{activeTrainer: NewMemoryActiveTrainerStore()}
	_, err := svc.resolveActiveTrainerID(t.Context(), "1", []TelegramTrainer{
		{TrainerID: uuid.New()},
		{TrainerID: uuid.New()},
	})
	if !errors.Is(err, ErrActiveTrainerNotSet) {
		t.Fatalf("err = %v", err)
	}
}

func TestPickProgramName_englishOnly(t *testing.T) {
	if got := pickProgramName("Eng", "  "); got != "Eng" {
		t.Fatalf("got = %q", got)
	}
}

func TestResolveActiveTrainerID_staleStoredID(t *testing.T) {
	store := NewMemoryActiveTrainerStore()
	stale := uuid.New()
	_ = store.Set(t.Context(), "1", stale)
	svc := &Service{activeTrainer: store}
	_, err := svc.resolveActiveTrainerID(t.Context(), "1", []TelegramTrainer{
		{TrainerID: uuid.New()},
		{TrainerID: uuid.New()},
	})
	if !errors.Is(err, ErrActiveTrainerNotSet) {
		t.Fatalf("err = %v", err)
	}
}
