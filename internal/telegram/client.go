package telegram

import (
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HTTPClient is the shared client for every call to Telegram, so a slow
// Telegram cannot hold a request or startup forever.
var HTTPClient = &http.Client{Timeout: 10 * time.Second}

// NewBotAPI builds a Bot API client without the getMe call that
// tgbotapi.NewBotAPI makes, so it cannot fail on the network. Self stays empty;
// the app only sends requests and receives updates by webhook.
func NewBotAPI(token string) *tgbotapi.BotAPI {
	api := &tgbotapi.BotAPI{Token: token, Client: HTTPClient, Buffer: 100}
	api.SetAPIEndpoint(tgbotapi.APIEndpoint)
	return api
}
