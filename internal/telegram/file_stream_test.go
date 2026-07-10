package telegram

import "testing"

func TestContentTypeForFilePath_allExtensions(t *testing.T) {
	cases := map[string]string{
		"photos/x.jpg":  "image/jpeg",
		"photos/x.png":  "image/png",
		"photos/x.webp": "image/webp",
		"photos/x.gif":  "image/gif",
		"photos/x":      "image/jpeg",
	}
	for path, want := range cases {
		if got := ContentTypeForFilePath(path); got != want {
			t.Fatalf("ContentTypeForFilePath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestBotFileURL(t *testing.T) {
	got := BotFileURL("token", "photos/file.jpg")
	want := "https://api.telegram.org/file/bottoken/photos/file.jpg"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}
