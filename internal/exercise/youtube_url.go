package exercise

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var youtubeVideoIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)

func validateYouTubeVideoURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("%w: video_url must be a valid YouTube HTTPS URL", ErrValidation)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: video_url must be a valid YouTube HTTPS URL", ErrValidation)
	}
	if !youtubeVideoIDFromURL(u) {
		return fmt.Errorf("%w: video_url must be a valid YouTube HTTPS URL", ErrValidation)
	}
	return nil
}

func youtubeVideoIDFromURL(u *url.URL) bool {
	switch strings.ToLower(u.Hostname()) {
	case "youtu.be", "www.youtu.be":
		return youtubeVideoIDRe.MatchString(firstPathSegment(u.Path))
	case "youtube.com", "www.youtube.com", "m.youtube.com":
		return youtubeVideoIDFromYouTubePath(u)
	default:
		return false
	}
}

func firstPathSegment(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	seg, _, _ := strings.Cut(path, "/")
	seg, _, _ = strings.Cut(seg, "?")
	return seg
}

func youtubeVideoIDFromYouTubePath(u *url.URL) bool {
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return false
	}
	parts := strings.Split(path, "/")
	switch parts[0] {
	case "watch":
		return youtubeVideoIDRe.MatchString(u.Query().Get("v"))
	case "embed", "shorts", "v", "live":
		if len(parts) < 2 {
			return false
		}
		return youtubeVideoIDRe.MatchString(parts[1])
	default:
		return false
	}
}
