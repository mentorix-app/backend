package trainerclient

import (
	"strings"
	"testing"
)

func Test_normalizeDisplayName(t *testing.T) {
	if got := normalizeDisplayName(""); got != "Client" {
		t.Fatalf("empty = %q", got)
	}
	if got := normalizeDisplayName("  Ivan  "); got != "Ivan" {
		t.Fatalf("trim = %q", got)
	}
}

func Test_inviteURL(t *testing.T) {
	got := inviteURL("@mentorix_bot", "abc123")
	want := "https://t.me/mentorix_bot?start=inv_abc123"
	if got != want {
		t.Fatalf("inviteURL = %q, want %q", got, want)
	}
}

func Test_newInviteToken_unique(t *testing.T) {
	a, err := newInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || strings.TrimSpace(a) == "" {
		t.Fatalf("tokens = %q %q", a, b)
	}
}
