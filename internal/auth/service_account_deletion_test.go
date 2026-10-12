package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (f *fakeAuthStore) DeleteClientAccount(_ context.Context, userID uuid.UUID) (string, error) {
	f.deleteCalls = append(f.deleteCalls, userID)
	if f.deleteErr != nil {
		return "", f.deleteErr
	}
	return f.deletedTelegramID, nil
}

// cleanupRecorder records how the cleanup was called and the state of its context
// at that moment.
type cleanupRecorder struct {
	calls       []string
	err         error
	ctxErr      error
	deadline    time.Time
	hasDeadline bool
}

func (r *cleanupRecorder) run(ctx context.Context, telegramUserID string) error {
	r.calls = append(r.calls, telegramUserID)
	r.ctxErr = ctx.Err()
	r.deadline, r.hasDeadline = ctx.Deadline()
	return r.err
}

// deletionService returns a service whose log lines land in the returned buffer.
func deletionService(store authStore, cleanup *cleanupRecorder) (*Service, *bytes.Buffer) {
	var logs bytes.Buffer
	svc := testAuthService(store)
	svc.log = slog.New(slog.NewJSONHandler(&logs, nil))
	if cleanup != nil {
		svc.telegramCleanup = cleanup.run
	}
	return svc, &logs
}

func TestService_DeleteAccount_deletesAClient(t *testing.T) {
	userID := uuid.New()
	store := &fakeAuthStore{}
	svc, _ := deletionService(store, nil)

	if err := svc.DeleteAccount(context.Background(), userID); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if len(store.deleteCalls) != 1 || store.deleteCalls[0] != userID {
		t.Fatalf("store delete calls = %v, want exactly [%v]", store.deleteCalls, userID)
	}
}

// Part 1 has no Apple revoker, so the store's refusal of an account with an Apple
// sign-in is told to the caller as a missing configuration.
func TestService_DeleteAccount_unrevokedAppleSignInIsNotConfigured(t *testing.T) {
	for _, storeErr := range []error{
		ErrAppleSignInNotRevoked,
		fmt.Errorf("lock: %w", ErrAppleSignInNotRevoked),
	} {
		cleanup := &cleanupRecorder{}
		store := &fakeAuthStore{deleteErr: storeErr, deletedTelegramID: "tg-1"}
		svc, logs := deletionService(store, cleanup)

		err := svc.DeleteAccount(context.Background(), uuid.New())

		if !errors.Is(err, ErrProviderNotConfigured) {
			t.Fatalf("store error %v: error = %v, want ErrProviderNotConfigured", storeErr, err)
		}
		if len(cleanup.calls) != 0 || logs.Len() != 0 {
			t.Errorf("cleanup calls = %v, log = %q, want neither", cleanup.calls, logs)
		}
	}
}

func TestService_DeleteAccount_otherStoreErrorsAreReturned(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name     string
		storeErr error
		want     error
	}{
		{"database failure", boom, boom},
		{"account gone", pgx.ErrNoRows, pgx.ErrNoRows},
		{"refused", ErrAccountDeletionUnavailable, ErrAccountDeletionUnavailable},
		{"refused by the database", fmt.Errorf("delete user: %w: %w", ErrAccountDeletionUnavailable, boom), ErrAccountDeletionUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAuthStore{deleteErr: tt.storeErr, deletedTelegramID: "tg-1"}
			cleanup := &cleanupRecorder{}
			svc, logs := deletionService(store, cleanup)

			err := svc.DeleteAccount(context.Background(), uuid.New())

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if errors.Is(err, ErrProviderNotConfigured) {
				t.Errorf("error %v is also ErrProviderNotConfigured", err)
			}
			if len(cleanup.calls) != 0 {
				t.Errorf("cleanup ran after a failure: %v", cleanup.calls)
			}
			if strings.Contains(logs.String(), "account deleted") {
				t.Errorf("logged a deletion that did not happen: %s", logs)
			}
		})
	}
}

func TestService_DeleteAccount_cleanupGetsTheTelegramID(t *testing.T) {
	store := &fakeAuthStore{deletedTelegramID: "tg-100"}
	cleanup := &cleanupRecorder{}
	svc, _ := deletionService(store, cleanup)

	if err := svc.DeleteAccount(context.Background(), uuid.New()); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if len(cleanup.calls) != 1 || cleanup.calls[0] != "tg-100" {
		t.Fatalf("cleanup calls = %q, want exactly [tg-100]", cleanup.calls)
	}
}

// A client that drops the connection right after the commit cancels the request
// context. The cleanup still has to run, on a context of its own with a deadline.
func TestService_DeleteAccount_cleanupSurvivesACancelledRequest(t *testing.T) {
	store := &fakeAuthStore{deletedTelegramID: "tg-100"}
	cleanup := &cleanupRecorder{}
	svc, logs := deletionService(store, cleanup)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := svc.DeleteAccount(ctx, uuid.New()); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if len(cleanup.calls) != 1 {
		t.Fatalf("cleanup calls = %q, want 1", cleanup.calls)
	}
	if cleanup.ctxErr != nil {
		t.Errorf("cleanup context error = %v, want a live context", cleanup.ctxErr)
	}
	if !cleanup.hasDeadline {
		t.Fatal("cleanup context has no deadline")
	}
	if left := time.Until(cleanup.deadline); left <= 0 || left > 5*time.Second {
		t.Errorf("cleanup deadline is in %v, want within 5s", left)
	}
	if strings.Contains(logs.String(), "WARN") {
		t.Errorf("log carries a warning: %s", logs)
	}
}

func TestService_DeleteAccount_cleanupIsSkippedWithoutTelegram(t *testing.T) {
	store := &fakeAuthStore{}
	cleanup := &cleanupRecorder{}
	svc, _ := deletionService(store, cleanup)

	if err := svc.DeleteAccount(context.Background(), uuid.New()); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	if len(cleanup.calls) != 0 {
		t.Fatalf("cleanup calls = %q, want none", cleanup.calls)
	}
}

func TestService_DeleteAccount_worksWithoutACleanup(t *testing.T) {
	store := &fakeAuthStore{deletedTelegramID: "tg-100"}
	svc, _ := deletionService(store, nil)

	if err := svc.DeleteAccount(context.Background(), uuid.New()); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
}

func logLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		out = append(out, rec)
	}
	return out
}

func TestService_DeleteAccount_failingCleanupDoesNotFailTheDeletion(t *testing.T) {
	userID := uuid.New()
	store := &fakeAuthStore{deletedTelegramID: "tg-secret-7"}
	cleanup := &cleanupRecorder{err: errors.New("redis down")}
	svc, logs := deletionService(store, cleanup)

	if err := svc.DeleteAccount(context.Background(), userID); err != nil {
		t.Fatalf("DeleteAccount() error = %v, want the deletion to succeed", err)
	}

	lines := logLines(t, logs)
	if len(lines) != 2 {
		t.Fatalf("log lines = %v, want a warning and the deletion line", lines)
	}
	if lines[0]["level"] != "WARN" || lines[0]["error"] != "redis down" {
		t.Errorf("first line = %v, want a warning that carries the cause", lines[0])
	}
	if lines[1]["level"] != "INFO" || lines[1]["msg"] != "account deleted" {
		t.Errorf("second line = %v, want the info line", lines[1])
	}
	if strings.Contains(logs.String(), "tg-secret-7") {
		t.Errorf("log carries the Telegram id: %s", logs)
	}
}

func TestService_DeleteAccount_logsOneInfoLineWithTheUserIDOnly(t *testing.T) {
	userID := uuid.New()
	store := &fakeAuthStore{
		deletedTelegramID: "tg-secret-7",
		profile:           UserProfile{Email: "secret@example.com", Name: "Secret Name"},
	}
	svc, logs := deletionService(store, &cleanupRecorder{})

	if err := svc.DeleteAccount(context.Background(), userID); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	lines := logLines(t, logs)
	if len(lines) != 1 {
		t.Fatalf("log lines = %v, want exactly one", lines)
	}
	rec := lines[0]
	if rec["level"] != "INFO" || rec["msg"] != "account deleted" || rec["user_id"] != userID.String() {
		t.Errorf("line = %v, want INFO \"account deleted\" with user_id %v", rec, userID)
	}
	for _, personal := range []string{"secret@example.com", "Secret Name", "tg-secret-7"} {
		if strings.Contains(logs.String(), personal) {
			t.Errorf("log carries %q: %s", personal, logs)
		}
	}
}

func TestNewService_loggerOptionKeepsTheDefaultForNil(t *testing.T) {
	if got := NewService(nil, "secret", 0, 0).log; got != slog.Default() {
		t.Errorf("default logger = %v, want slog.Default()", got)
	}
	if got := NewService(nil, "secret", 0, 0, WithLogger(nil)).log; got != slog.Default() {
		t.Errorf("logger after WithLogger(nil) = %v, want slog.Default()", got)
	}
	custom := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if got := NewService(nil, "secret", 0, 0, WithLogger(custom)).log; got != custom {
		t.Errorf("logger after WithLogger(custom) = %v, want the custom one", got)
	}
}
