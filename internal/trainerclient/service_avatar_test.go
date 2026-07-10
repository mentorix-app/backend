package trainerclient_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"mentorix-backend/internal/trainerclient"
)

type stubPhotos struct {
	path string
	ok   bool
	err  error
}

func (s stubPhotos) ProfilePhotoFilePath(context.Context, string) (string, bool, error) {
	return s.path, s.ok, s.err
}

func TestService_WithAvatarSupport(t *testing.T) {
	svc := trainerclient.NewService(nil, nil, trainerclient.InviteSettings{}, trainerclient.NewMemoryActiveTrainerStore(), nil,
		trainerclient.WithAvatarSupport("test-jwt-secret-at-least-32-chars-long", "123:abc", stubPhotos{path: "photos/a.jpg", ok: true}),
	)
	url := trainerclient.BuildAvatarURL(uuid.New(), "photos/a.jpg", "test-jwt-secret-at-least-32-chars-long")
	if url == "" {
		t.Fatal("expected avatar url")
	}
	_ = svc
}
