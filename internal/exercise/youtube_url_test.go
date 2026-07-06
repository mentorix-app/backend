package exercise

import "testing"

func TestValidateYouTubeVideoURL(t *testing.T) {
	valid := []string{
		"",
		"   ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtube.com/watch?v=dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://www.youtube.com/embed/dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ",
		"https://www.youtube.com/live/dQw4w9WgXcQ",
	}
	for _, raw := range valid {
		if err := validateYouTubeVideoURL(raw); err != nil {
			t.Fatalf("validateYouTubeVideoURL(%q) error = %v, want nil", raw, err)
		}
	}

	invalid := []string{
		"http://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://vimeo.com/123456789",
		"https://www.youtube.com/watch?v=short",
		"https://youtu.be/",
		"https://example.com/watch?v=dQw4w9WgXcQ",
		"not-a-url",
		"https://www.youtube.com/playlist?list=PLtest",
	}
	for _, raw := range invalid {
		if err := validateYouTubeVideoURL(raw); err == nil {
			t.Fatalf("validateYouTubeVideoURL(%q) = nil, want error", raw)
		}
	}
}
