package telegram

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

const testBotToken = "123456:SECRET-token"

func TestStripURL(t *testing.T) {
	plain := errors.New("plain failure")

	t.Run("nil", func(t *testing.T) {
		if got := StripURL(nil); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("plain error unchanged", func(t *testing.T) {
		if got := StripURL(plain); got != plain {
			t.Fatalf("got %v, want the same error", got)
		}
	})

	tests := []struct {
		name  string
		err   error
		inner error
	}{
		{
			name:  "url error",
			err:   &url.Error{Op: "Get", URL: "https://api.telegram.org/bot" + testBotToken + "/getMe", Err: context.DeadlineExceeded},
			inner: context.DeadlineExceeded,
		},
		{
			name:  "wrapped url error",
			err:   errors.Join(errors.New("ctx"), &url.Error{Op: "Post", URL: "https://api.telegram.org/bot" + testBotToken + "/setWebhook", Err: plain}),
			inner: plain,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripURL(tt.err)
			if strings.Contains(got.Error(), testBotToken) || strings.Contains(got.Error(), "api.telegram.org") {
				t.Fatalf("text still has the URL: %q", got)
			}
			if !errors.Is(got, tt.inner) {
				t.Fatalf("errors.Is lost the inner error: %v", got)
			}
		})
	}

	t.Run("keeps op and inner text", func(t *testing.T) {
		got := StripURL(&url.Error{Op: "Get", URL: "https://x/bot" + testBotToken, Err: plain})
		if got.Error() != "Get: plain failure" {
			t.Fatalf("got %q", got)
		}
	})
}
