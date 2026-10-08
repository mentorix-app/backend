package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/telegram"
)

const telegramWebhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

var telegramAPIBase = "https://api.telegram.org/bot%s/setWebhook"

type WebhookConfig struct {
	URL         string
	SecretToken string
}

type setWebhookResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func RegisterWebhook(ctx context.Context, botToken string, cfg WebhookConfig) error {
	token := strings.TrimSpace(botToken)
	url := strings.TrimSpace(cfg.URL)
	if token == "" {
		return fmt.Errorf("bot token is required")
	}
	if url == "" {
		return fmt.Errorf("webhook url is required")
	}

	payload := map[string]any{
		"url":                  url,
		"drop_pending_updates": false,
		"max_connections":      40,
	}
	if secret := strings.TrimSpace(cfg.SecretToken); secret != "" {
		payload["secret_token"] = secret
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal setWebhook: %w", err)
	}

	endpoint := fmt.Sprintf(telegramAPIBase, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build setWebhook request: %w", telegram.StripURL(err))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := telegram.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("set webhook request: %w", telegram.StripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read setWebhook response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("set webhook: status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out setWebhookResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return fmt.Errorf("decode setWebhook response: %w", err)
	}
	if !out.OK {
		if out.Description != "" {
			return fmt.Errorf("set webhook: %s", out.Description)
		}
		return fmt.Errorf("set webhook failed")
	}
	return nil
}

func WebhookHandler(secret string, bot *Bot) echo.HandlerFunc {
	secret = strings.TrimSpace(secret)
	return func(c echo.Context) error {
		if secret != "" && c.Request().Header.Get(telegramWebhookSecretHeader) != secret {
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}

		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
		}

		var update tgbotapi.Update
		if err := json.Unmarshal(body, &update); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid json")
		}

		bot.HandleUpdate(c.Request().Context(), update)
		return c.NoContent(http.StatusOK)
	}
}

func (b *Bot) RegisterWebhook(ctx context.Context, botToken string, cfg WebhookConfig) error {
	return RegisterWebhook(ctx, botToken, cfg)
}

// RegisterWebhookWithRetry calls register until it succeeds or ctx is done,
// waiting pause between attempts. It never exits the process.
func RegisterWebhookWithRetry(ctx context.Context, logger *slog.Logger, pause time.Duration, url string, register func(context.Context) error) {
	for {
		err := register(ctx)
		if err == nil {
			logger.Info("telegram webhook registered", "url", url)
			return
		}
		if ctx.Err() != nil {
			return
		}
		logger.Error("telegram webhook registration failed; will retry", "url", url, "error", err, "retry_in", pause)
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}
}
