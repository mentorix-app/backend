package telegram

import (
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

func NewProfilePhotoClient(token string) *ProfilePhotoClient {
	return &ProfilePhotoClient{api: NewBotAPI(token)}
}

func newProfilePhotoClient(api profilePhotoAPI) *ProfilePhotoClient {
	return &ProfilePhotoClient{api: api}
}
