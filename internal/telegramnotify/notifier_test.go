package telegramnotify_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"mentorix-backend/internal/db/sqlc"
	"mentorix-backend/internal/telegramnotify"
)

type recordingSender struct {
	mu     sync.Mutex
	chatID int64
	text   string
	calls  int
	err    error
}

func (r *recordingSender) SendMessage(_ context.Context, chatID int64, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.chatID = chatID
	r.text = text
	return r.err
}

// syncBuffer is a log sink that is safe for concurrent writers.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newTestLogger() (*slog.Logger, *syncBuffer) {
	buf := &syncBuffer{}
	return slog.New(slog.NewTextHandler(buf, nil)), buf
}

// drain waits until every queued notification has been handled.
func drain(t *testing.T, n *telegramnotify.Notifier) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := n.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
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
	sender := telegramnotify.NewSender("")
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
	drain(t, n)
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
	drain(t, n)
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
	drain(t, n)
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d", sender.calls)
	}
}

func TestNotifier_invalidTelegramSubject(t *testing.T) {
	log, logs := newTestLogger()
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{subject: "not-a-number"}, sender, log)

	if err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("NotifyProgramAssigned: %v", err)
	}
	drain(t, n)
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
	if !strings.Contains(logs.String(), "telegram notify lookup failed") {
		t.Fatalf("logs = %q", logs.String())
	}
}

func TestNotifier_sendErrorLogged(t *testing.T) {
	log, logs := newTestLogger()
	sender := &recordingSender{err: errors.New("telegram down")}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subject:     "1",
		trainerName: "Иван",
		version:     sqlc.GetProgramVersionDisplayByIDRow{NameRu: "Сила"},
	}, sender, log)

	if err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("NotifyProgramAssigned: %v", err)
	}
	drain(t, n)
	if !strings.Contains(logs.String(), "telegram notify failed") || !strings.Contains(logs.String(), "telegram down") {
		t.Fatalf("logs = %q", logs.String())
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
	drain(t, n)
	if !strings.Contains(sender.text, "программа") {
		t.Fatalf("text = %q", sender.text)
	}
}

func TestNotifier_notifyWorkoutCommented_success(t *testing.T) {
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subject:     "777",
		trainerName: "Иван",
	}, sender, nil)

	err := n.NotifyWorkoutCommented(context.Background(), uuid.New(), uuid.New(), telegramnotify.WorkoutComment{
		ProgramName:   "Strength",
		ProgramNameRu: "Сила",
		WeekNumber:    2,
		DayNumber:     3,
		ResultText:    "присед 5х5 90 кг",
		CommentText:   "Отличная работа!",
	})
	if err != nil {
		t.Fatalf("NotifyWorkoutCommented: %v", err)
	}
	drain(t, n)
	if sender.calls != 1 || sender.chatID != 777 {
		t.Fatalf("sender = %+v", sender)
	}
	want := "Тренер Иван ответил на ваш результат тренировки «Сила», неделя 2, день 3.\n\n" +
		"Ваш результат:\nприсед 5х5 90 кг\n\n" +
		"Ответ тренера:\nОтличная работа!"
	if sender.text != want {
		t.Fatalf("text = %q", sender.text)
	}
}

func TestNotifier_notifyWorkoutCommented_skipsWithoutIdentity(t *testing.T) {
	sender := &recordingSender{}
	n := telegramnotify.NewNotifierWithStore(&fakeNotifyStore{
		subjectErr: pgx.ErrNoRows,
	}, sender, nil)

	err := n.NotifyWorkoutCommented(context.Background(), uuid.New(), uuid.New(), telegramnotify.WorkoutComment{CommentText: "ok"})
	if err != nil {
		t.Fatalf("NotifyWorkoutCommented: %v", err)
	}
	drain(t, n)
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
}

func TestNotifier_notifyWorkoutCommented_nilSenderNoop(t *testing.T) {
	n := telegramnotify.NewNotifierWithStore(nil, nil, nil)
	err := n.NotifyWorkoutCommented(context.Background(), uuid.New(), uuid.New(), telegramnotify.WorkoutComment{CommentText: "ok"})
	if err != nil {
		t.Fatalf("NotifyWorkoutCommented: %v", err)
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
	drain(t, n)
	if !strings.Contains(sender.text, "ваш тренер") {
		t.Fatalf("text = %q", sender.text)
	}
}

func TestNotifier_storeErrorsAreLogged(t *testing.T) {
	cases := map[string]*fakeNotifyStore{
		"subject lookup": {subjectErr: errors.New("db")},
		"trainer lookup": {subject: "1", trainerErr: errors.New("db")},
		"version lookup": {subject: "1", trainerName: "Иван", versionErr: errors.New("db")},
	}
	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			log, logs := newTestLogger()
			sender := &recordingSender{}
			n := telegramnotify.NewNotifierWithStore(store, sender, log)
			if err := n.NotifyProgramAssigned(context.Background(), uuid.New(), uuid.New(), uuid.New()); err != nil {
				t.Fatalf("NotifyProgramAssigned: %v", err)
			}
			drain(t, n)
			if sender.calls != 0 {
				t.Fatalf("sender calls = %d, want 0", sender.calls)
			}
			if !strings.Contains(logs.String(), "telegram notify lookup failed") {
				t.Fatalf("logs = %q", logs.String())
			}
		})
	}
}
