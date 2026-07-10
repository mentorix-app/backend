package telegram

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type profilePhotoAPI interface {
	GetUserProfilePhotos(config tgbotapi.UserProfilePhotosConfig) (tgbotapi.UserProfilePhotos, error)
	GetFile(config tgbotapi.FileConfig) (tgbotapi.File, error)
}

// ProfilePhotoClient fetches Telegram user profile photo metadata via Bot API.
type ProfilePhotoClient struct {
	api profilePhotoAPI
}

func NewProfilePhotoClient(token string) (*ProfilePhotoClient, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot api: %w", err)
	}
	return &ProfilePhotoClient{api: api}, nil
}

func newProfilePhotoClient(api profilePhotoAPI) *ProfilePhotoClient {
	return &ProfilePhotoClient{api: api}
}
