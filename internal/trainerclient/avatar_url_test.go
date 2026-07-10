package trainerclient

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBuildAvatarURL_emptyWithoutFilePath(t *testing.T) {
	if got := BuildAvatarURL(uuid.New(), "", "secret"); got != "" {
		t.Fatalf("url = %q, want empty", got)
	}
	if got := BuildAvatarURL(uuid.New(), "photos/a.jpg", ""); got != "" {
		t.Fatalf("url = %q, want empty without secret", got)
	}
}

func TestVerifyAvatarURL_roundTrip(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-chars-long"
	clientID := uuid.MustParse("89f7ebd6-1111-2222-3333-444444444444")
	url := BuildAvatarURL(clientID, "photos/file.jpg", secret)
	if url == "" {
		t.Fatal("expected url")
	}
	if !strings.Contains(url, clientID.String()) {
		t.Fatalf("url = %q", url)
	}

	exp := time.Now().Add(avatarURLTTL).Unix()
	sig := signAvatarURL(secret, clientID, exp)
	if !VerifyAvatarURL(secret, clientID, exp, sig) {
		t.Fatal("expected valid sig")
	}
}

func TestVerifyAvatarURL_expired(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-chars-long"
	clientID := uuid.New()
	exp := time.Now().Add(-time.Hour).Unix()
	sig := signAvatarURL(secret, clientID, exp)
	if VerifyAvatarURL(secret, clientID, exp, sig) {
		t.Fatal("expected expired sig to fail")
	}
}

func TestVerifyAvatarURL_invalidSig(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-chars-long"
	clientID := uuid.New()
	exp := time.Now().Add(time.Hour).Unix()
	if VerifyAvatarURL(secret, clientID, exp, "bad") {
		t.Fatal("expected invalid sig to fail")
	}
	if VerifyAvatarURL("", clientID, exp, signAvatarURL(secret, clientID, exp)) {
		t.Fatal("expected empty secret to fail")
	}
}

func TestVerifyAvatarURL_tamperedClient(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-chars-long"
	clientA := uuid.New()
	clientB := uuid.New()
	exp := time.Now().Add(time.Hour).Unix()
	sig := signAvatarURL(secret, clientA, exp)
	if VerifyAvatarURL(secret, clientB, exp, sig) {
		t.Fatal("expected tampered client id to fail")
	}
}
