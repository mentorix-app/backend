package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/telegram"
)

func TestWebhookHandler_rejectsBadSecret(t *testing.T) {
	e := echo.New()
	bot := New(&noopTelegramAPI{}, &fakeTrainerClient{})
	e.POST("/telegram/webhook", WebhookHandler("secret", bot))

	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWebhookHandler_acceptsUpdate(t *testing.T) {
	e := echo.New()
	sender := &recordingTelegramAPI{}
	bot := New(sender, &fakeTrainerClient{})
	e.POST("/telegram/webhook", WebhookHandler("secret", bot))

	body := `{"update_id":1,"message":{"message_id":1,"text":"/help","chat":{"id":42},"from":{"id":99,"first_name":"Test"}}}`
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", bytes.NewReader([]byte(body)))
	req.Header.Set(telegramWebhookSecretHeader, "secret")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if sender.sendCalls == 0 {
		t.Fatal("expected bot to send a reply")
	}
}

func TestRegisterWebhook_requiresURL(t *testing.T) {
	err := RegisterWebhook(t.Context(), "token", WebhookConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterWebhook_requiresToken(t *testing.T) {
	err := RegisterWebhook(t.Context(), "", WebhookConfig{URL: "https://example.com/hook"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterWebhook_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123/setWebhook" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if got, ok := payload["drop_pending_updates"]; ok && got != false {
			t.Errorf("drop_pending_updates = %v, want false or absent", got)
		}
		_ = json.NewEncoder(w).Encode(setWebhookResponse{OK: true})
	}))
	defer srv.Close()

	old := telegramAPIBase
	telegramAPIBase = srv.URL + "/bot%s/setWebhook"
	t.Cleanup(func() { telegramAPIBase = old })

	err := RegisterWebhook(t.Context(), "123", WebhookConfig{URL: "https://example.com/hook", SecretToken: "sec"})
	if err != nil {
		t.Fatalf("RegisterWebhook: %v", err)
	}
}

func TestRegisterWebhook_telegramError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(setWebhookResponse{OK: false, Description: "bad token"})
	}))
	defer srv.Close()

	old := telegramAPIBase
	telegramAPIBase = srv.URL + "/bot%s/setWebhook"
	t.Cleanup(func() { telegramAPIBase = old })

	err := RegisterWebhook(t.Context(), "123", WebhookConfig{URL: "https://example.com/hook"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWebhookHandler_invalidJSON(t *testing.T) {
	e := echo.New()
	bot := New(&noopTelegramAPI{}, &fakeTrainerClient{})
	e.POST("/telegram/webhook", WebhookHandler("secret", bot))

	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", bytes.NewReader([]byte(`{`)))
	req.Header.Set(telegramWebhookSecretHeader, "secret")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

type recordingTelegramAPI struct {
	sendCalls int
}

func (r *recordingTelegramAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	r.sendCalls++
	return tgbotapi.Message{}, nil
}

func (r *recordingTelegramAPI) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

type noopTelegramAPI struct{}

func (noopTelegramAPI) Send(tgbotapi.Chattable) (tgbotapi.Message, error) {
	return tgbotapi.Message{}, nil
}

func (noopTelegramAPI) Request(tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func TestHandleUpdate_callbackQuery(t *testing.T) {
	sender := &recordingTelegramAPI{}
	backend := &fakeTrainerClient{}
	bot := New(sender, backend)
	update := tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb1",
			Data: "noop",
			From: &tgbotapi.User{ID: 1},
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: 42},
			},
		},
	}
	bot.HandleUpdate(t.Context(), update)
	_ = json.Valid([]byte(`{"update_id":1}`))
}

func TestRegisterWebhook_requestErrorHasNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close() // connection refused

	old := telegramAPIBase
	telegramAPIBase = base + "/bot%s/setWebhook"
	t.Cleanup(func() { telegramAPIBase = old })

	const token = "123456:SECRET-token"
	err := RegisterWebhook(t.Context(), token, WebhookConfig{URL: "https://example.com/hook"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaks the bot token: %q", err)
	}
}

func TestRegisterWebhook_timesOutOnHangingServer(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)

	old := telegramAPIBase
	telegramAPIBase = srv.URL + "/bot%s/setWebhook"
	oldClient := telegram.HTTPClient
	telegram.HTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	t.Cleanup(func() {
		telegramAPIBase = old
		telegram.HTTPClient = oldClient
	})

	start := time.Now()
	err := RegisterWebhook(t.Context(), "123", WebhookConfig{URL: "https://example.com/hook"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("took %v, want about the client timeout", elapsed)
	}
}

func TestRegisterWebhookWithRetry_retriesUntilSuccess(t *testing.T) {
	calls := 0
	register := func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("telegram down")
		}
		return nil
	}

	RegisterWebhookWithRetry(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), 0, "https://example.test/hook", register)

	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRegisterWebhookWithRetry_stopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	register := func(context.Context) error {
		calls++
		cancel()
		return errors.New("telegram down")
	}

	done := make(chan struct{})
	go func() {
		RegisterWebhookWithRetry(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Hour, "https://example.test/hook", register)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not return after cancel")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
