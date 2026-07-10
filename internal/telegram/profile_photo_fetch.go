package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ProfilePhotoFilePath returns the largest profile photo file path, or ("", false, nil) if none.
func (c *ProfilePhotoClient) ProfilePhotoFilePath(ctx context.Context, telegramUserID string) (string, bool, error) {
	_ = ctx

	userID, err := strconv.ParseInt(strings.TrimSpace(telegramUserID), 10, 64)
	if err != nil || userID <= 0 {
		return "", false, fmt.Errorf("invalid telegram user id: %q", telegramUserID)
	}
	if c == nil || c.api == nil {
		return "", false, nil
	}

	photos, err := c.api.GetUserProfilePhotos(tgbotapi.UserProfilePhotosConfig{
		UserID: userID,
		Limit:  1,
	})
	if err != nil {
		return "", false, fmt.Errorf("get user profile photos: %w", err)
	}
	if photos.TotalCount == 0 || len(photos.Photos) == 0 || len(photos.Photos[0]) == 0 {
		return "", false, nil
	}

	sizes := photos.Photos[0]
	largest := sizes[0]
	for _, size := range sizes[1:] {
		if size.Width*size.Height > largest.Width*largest.Height {
			largest = size
		}
	}

	file, err := c.api.GetFile(tgbotapi.FileConfig{FileID: largest.FileID})
	if err != nil {
		return "", false, fmt.Errorf("get profile photo file: %w", err)
	}
	if file.FilePath == "" {
		return "", false, nil
	}
	return file.FilePath, true, nil
}
