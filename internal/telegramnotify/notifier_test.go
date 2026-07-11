package telegramnotify_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/telegramnotify"
)

type recordingSender struct {
	chatID int64
	text   string
	calls  int
	err    error
}

func (r *recordingSender) SendMessage(_ context.Context, chatID int64, text string) error {
	r.calls++
	r.chatID = chatID
	r.text = text
	return r.err
}

type fakeNotifyStore struct {
	subject     string
	subjectErr  error
	trainerName string
	trainerErr  error
	version     sqlc.GetProgramVersionDisplayByIDRow
	versionErr  error
}

func (f *fakeNotifyStore) GetTelegramSubjectByUserID(context.Context, sqlc.GetTelegramSubjectByUserIDParams) (string, error) {
	return f.subject, f.subjectErr
}

func (f *fakeNotifyStore) GetTrainerDisplayNameByTrainerID(context.Context, pgtype.UUID) (string, error) {
	return f.trainerName, f.trainerErr
}

func (f *fakeNotifyStore) GetProgramVersionDisplayByID(context.Context, pgtype.UUID) (sqlc.GetProgramVersionDisplayByIDRow, error) {
	return f.version, f.versionErr
}

func TestNewSender_emptyTokenIsNoop(t *testing.T) {
	sender, err := telegramnotify.NewSender("")
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	if err := sender.SendMessage(context.Background(), 123, "hi"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
}

func TestNotifier_nilSenderNoop(t *testing.T) {
	n := telegramnotify.NewNotifierWithStore(nil, nil, nil)
	if err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("NotifyProgramAssigned: %v", err)
	}
}

func TestNotifier_skipsWithoutTelegramIdentity(t *testing.T) {
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subjectErr: pgx.ErrNoRows,
	}, sender, nil)

	err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("NotifyProgramAssigned: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
}

func TestNotifier_notifyAssigned_success(t *testing.T) {
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subject:     "424242",
		trainerName: "Иван",
		version:     sqlc.GetProgramVersionDisplayByIDRow{Name: "Strength", NameRu: "Сила"},
	}, sender, nil)

	clientID := uuid.New()
	trainerID := uuid.New()
	versionID := uuid.New()
	if err := n.NotifyProgramAssigned(context.Background(), clientID, trainerID, versionID); err != nil {
		t.Fatalf("NotifyProgramAssigned: %v", err)
	}
	if sender.calls != 1 || sender.chatID != 424242 {
		t.Fatalf("sender = %+v", sender)
	}
	want := "Тренер Иван назначил вам программу «Сила».\n\nОткройте «Программа» в меню."
	if sender.text != want {
		t.Fatalf("text = %q", sender.text)
	}
}

func TestNotifier_notifySynced_success(t *testing.T) {
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subject:     "100",
		trainerName: "Анна",
		version:     sqlc.GetProgramVersionDisplayByIDRow{Name: "Plan"},
	}, sender, nil)

	if err := n.NotifyProgramSynced(context.Background(), uuid.New(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("NotifyProgramSynced: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d", sender.calls)
	}
}

func TestNotifier_invalidTelegramSubject(t *testing.T) {
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subject: "not-a-number",
	}, &recordingSender{}, nil)

	err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error for invalid subject")
	}
}

func TestNotifier_sendErrorReturned(t *testing.T) {
	wantErr := errors.New("telegram down")
	sender := &recordingSender{err: wantErr}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subject:     "1",
		trainerName: "Иван",
		version:     sqlc.GetProgramVersionDisplayByIDRow{NameRu: "Сила"},
	}, sender, nil)

	err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New())
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestNotifier_versionFallbackName(t *testing.T) {
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subject:     "1",
		trainerName: "Иван",
		versionErr:  pgx.ErrNoRows,
	}, sender, nil)

	if err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("NotifyProgramAssigned: %v", err)
	}
	if !strings.Contains(sender.text, "программа") {
		t.Fatalf("text = %q", sender.text)
	}
}

func TestNewSender_invalidToken(t *testing.T) {
	_, err := telegramnotify.NewSender("invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestNotifier_trainerLookupUsesFallback(t *testing.T) {
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subject:    "9",
		trainerErr: pgx.ErrNoRows,
		version:    sqlc.GetProgramVersionDisplayByIDRow{NameRu: "Сила"},
	}, sender, nil)

	if err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("NotifyProgramAssigned: %v", err)
	}
	if !strings.Contains(sender.text, "ваш тренер") {
		t.Fatalf("text = %q", sender.text)
	}
}

func TestNotifier_storeErrors(t *testing.T) {
	ctx := context.Background()
	ids := func() (uuid.UUID, uuid.UUID, uuid.UUID) { return uuid.New(), uuid.New(), uuid.New() }

	t.Run("subject lookup", func(t *testing.T) {
		n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{subjectErr: errors.New("db")}, &recordingSender{}, nil)
		c, tr, v := ids()
		if err := n.NotifyProgramAssigned(ctx, c, tr, v); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("trainer lookup", func(t *testing.T) {
		n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
			subject:    "1",
			trainerErr: errors.New("db"),
		}, &recordingSender{}, nil)
		c, tr, v := ids()
		if err := n.NotifyProgramAssigned(ctx, c, tr, v); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("version lookup", func(t *testing.T) {
		n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
			subject:     "1",
			trainerName: "Иван",
			versionErr:  errors.New("db"),
		}, &recordingSender{}, nil)
		c, tr, v := ids()
		if err := n.NotifyProgramAssigned(ctx, c, tr, v); err == nil {
			t.Fatal("expected error")
		}
	})
}
