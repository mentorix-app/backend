package telegramnotify

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type noopSender struct{}

func (noopSender) SendMessage(context.Context, int64, string) error {
	return nil
}

type botAPISender struct {
	api *tgbotapi.BotAPI
}

func (s *botAPISender) SendMessage(_ context.Context, chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := s.api.Send(msg); err != nil {
		return fmt.Errorf("telegram sendMessage: %w", err)
	}
	return nil
}

func NewSender(botToken string) (Sender, error) {
	if botToken == "" {
		return noopSender{}, nil
	}
	api, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("telegram bot api: %w", err)
	}
	api.Debug = false
	return &botAPISender{api: api}, nil
}
