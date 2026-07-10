package telegram

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

func BotFileURL(botToken, filePath string) string {
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botToken, filePath)
}

func ContentTypeForFilePath(filePath string) string {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

func StreamBotFile(w http.ResponseWriter, client *http.Client, botToken, filePath string) error {
	if strings.TrimSpace(filePath) == "" || strings.TrimSpace(botToken) == "" {
		return ErrFileNotFound
	}
	if client == nil {
		client = http.DefaultClient
	}

	return streamHTTPFile(w, client, BotFileURL(botToken, filePath), filePath)
}

func streamHTTPFile(w http.ResponseWriter, client *http.Client, fileURL, filePath string) error {
	resp, err := client.Get(fileURL)
	if err != nil {
		return fmt.Errorf("fetch telegram file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return ErrFileNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch telegram file: status %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = ContentTypeForFilePath(filePath)
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, err = io.Copy(w, resp.Body)
	return err
}
