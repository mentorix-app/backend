package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestStreamBotFile_missingInput(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := StreamBotFile(rec, nil, "", "photos/a.jpg"); err != ErrFileNotFound {
		t.Fatalf("err = %v", err)
	}
	if err := StreamBotFile(rec, nil, "token", "  "); err != ErrFileNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamHTTPFile_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()

	rec := httptest.NewRecorder()
	if err := streamHTTPFile(rec, srv.Client(), srv.URL, "photos/a.png"); err != nil {
		t.Fatalf("streamHTTPFile: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q", ct)
	}
	if body := rec.Body.String(); body != "png-bytes" {
		t.Fatalf("body = %q", body)
	}
}

func TestStreamHTTPFile_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	rec := httptest.NewRecorder()
	if err := streamHTTPFile(rec, srv.Client(), srv.URL, "photos/missing.jpg"); err != ErrFileNotFound {
		t.Fatalf("err = %v, want ErrFileNotFound", err)
	}
}

func TestStreamHTTPFile_badStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	rec := httptest.NewRecorder()
	err := streamHTTPFile(rec, srv.Client(), srv.URL, "photos/a.jpg")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v, want status error", err)
	}
}

func TestProfilePhotoFilePath_nilClient(t *testing.T) {
	var c *ProfilePhotoClient
	path, ok, err := c.ProfilePhotoFilePath(context.Background(), "42")
	if err != nil || ok || path != "" {
		t.Fatalf("path=%q ok=%v err=%v", path, ok, err)
	}
}

func TestProfilePhotoFilePath_invalidUserID(t *testing.T) {
	c := &ProfilePhotoClient{}
	_, ok, err := c.ProfilePhotoFilePath(context.Background(), "not-a-number")
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestProfilePhotoFilePath_nilAPI(t *testing.T) {
	c := &ProfilePhotoClient{}
	path, ok, err := c.ProfilePhotoFilePath(context.Background(), "42")
	if err != nil || ok || path != "" {
		t.Fatalf("path=%q ok=%v err=%v", path, ok, err)
	}
}

func TestNewProfilePhotoClient_invalidToken(t *testing.T) {
	_, err := NewProfilePhotoClient("not-a-valid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

type fakeProfilePhotoAPI struct {
	photos  tgbotapi.UserProfilePhotos
	file    tgbotapi.File
	err     error
	fileErr error
}

func (f fakeProfilePhotoAPI) GetUserProfilePhotos(tgbotapi.UserProfilePhotosConfig) (tgbotapi.UserProfilePhotos, error) {
	if f.err != nil {
		return tgbotapi.UserProfilePhotos{}, f.err
	}
	return f.photos, nil
}

func (f fakeProfilePhotoAPI) GetFile(tgbotapi.FileConfig) (tgbotapi.File, error) {
	if f.fileErr != nil {
		return tgbotapi.File{}, f.fileErr
	}
	return f.file, nil
}

func TestProfilePhotoFilePath_success(t *testing.T) {
	c := newProfilePhotoClient(fakeProfilePhotoAPI{
		photos: tgbotapi.UserProfilePhotos{
			TotalCount: 1,
			Photos: [][]tgbotapi.PhotoSize{
				{
					{FileID: "small", Width: 10, Height: 10},
					{FileID: "large", Width: 100, Height: 100},
				},
			},
		},
		file: tgbotapi.File{FilePath: "photos/large.jpg"},
	})
	path, ok, err := c.ProfilePhotoFilePath(context.Background(), "42")
	if err != nil || !ok || path != "photos/large.jpg" {
		t.Fatalf("path=%q ok=%v err=%v", path, ok, err)
	}
}

func TestProfilePhotoFilePath_noPhotos(t *testing.T) {
	c := newProfilePhotoClient(fakeProfilePhotoAPI{})
	path, ok, err := c.ProfilePhotoFilePath(context.Background(), "42")
	if err != nil || ok || path != "" {
		t.Fatalf("path=%q ok=%v err=%v", path, ok, err)
	}
}

func TestProfilePhotoFilePath_apiError(t *testing.T) {
	c := newProfilePhotoClient(fakeProfilePhotoAPI{err: http.ErrHandlerTimeout})
	_, ok, err := c.ProfilePhotoFilePath(context.Background(), "42")
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestProfilePhotoFilePath_getFileError(t *testing.T) {
	c := newProfilePhotoClient(fakeProfilePhotoAPI{
		photos: tgbotapi.UserProfilePhotos{
			TotalCount: 1,
			Photos:     [][]tgbotapi.PhotoSize{{{FileID: "id", Width: 1, Height: 1}}},
		},
		fileErr: http.ErrHandlerTimeout,
	})
	_, ok, err := c.ProfilePhotoFilePath(context.Background(), "42")
	if err == nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestProfilePhotoFilePath_emptyFilePath(t *testing.T) {
	c := newProfilePhotoClient(fakeProfilePhotoAPI{
		photos: tgbotapi.UserProfilePhotos{
			TotalCount: 1,
			Photos:     [][]tgbotapi.PhotoSize{{{FileID: "id", Width: 1, Height: 1}}},
		},
		file: tgbotapi.File{},
	})
	path, ok, err := c.ProfilePhotoFilePath(context.Background(), "42")
	if err != nil || ok || path != "" {
		t.Fatalf("path=%q ok=%v err=%v", path, ok, err)
	}
}

func TestStreamBotFile_fetchError(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, http.ErrHandlerTimeout
	})}
	rec := httptest.NewRecorder()
	err := StreamBotFile(rec, client, "token", "photos/a.jpg")
	if err == nil {
		t.Fatal("expected fetch error")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
