package telegrambot

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/labstack/echo/v4"
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
	err := RegisterWebhook("token", WebhookConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterWebhook_requiresToken(t *testing.T) {
	err := RegisterWebhook("", WebhookConfig{URL: "https://example.com/hook"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterWebhook_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123/setWebhook" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(setWebhookResponse{OK: true})
	}))
	defer srv.Close()

	old := telegramAPIBase
	telegramAPIBase = srv.URL + "/bot%s/setWebhook"
	t.Cleanup(func() { telegramAPIBase = old })

	err := RegisterWebhook("123", WebhookConfig{URL: "https://example.com/hook", SecretToken: "sec"})
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

	err := RegisterWebhook("123", WebhookConfig{URL: "https://example.com/hook"})
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
