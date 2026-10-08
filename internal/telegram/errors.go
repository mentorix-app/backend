package telegram

import (
	"errors"
	"fmt"
	"net/url"
)

var ErrFileNotFound = errors.New("telegram file not found")

// StripURL returns err without any request URL in its text. Telegram URLs
// contain the bot token, and net/http errors (*url.Error) print the full URL.
// The inner error stays reachable through errors.Is and errors.As.
func StripURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s: %w", urlErr.Op, urlErr.Err)
	}
	return err
}
