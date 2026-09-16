package analytics

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"mentorix-backend/internal/trainerclient"
)

const testLinkSecret = "test-jwt-secret-at-least-32-chars-long"

func parseLink(t *testing.T, link string) (clientUserID, trainerID uuid.UUID, exp int64, sig string) {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	q := u.Query()
	clientUserID, err = uuid.Parse(q.Get("client_user_id"))
	if err != nil {
		t.Fatalf("client_user_id: %v", err)
	}
	trainerID, err = uuid.Parse(q.Get("trainer_id"))
	if err != nil {
		t.Fatalf("trainer_id: %v", err)
	}
	exp, err = strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil {
		t.Fatalf("exp: %v", err)
	}
	return clientUserID, trainerID, exp, q.Get("sig")
}

func TestClientAnalyticsLink_roundTrip(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	clientID, trainerID := uuid.New(), uuid.New()

	link := BuildClientAnalyticsLink("https://app.example.com/stats", testLinkSecret, clientID, trainerID, now)
	if !strings.HasPrefix(link, "https://app.example.com/stats?client_user_id=") {
		t.Fatalf("link = %q", link)
	}
	gotClient, gotTrainer, exp, sig := parseLink(t, link)
	if gotClient != clientID || gotTrainer != trainerID {
		t.Fatalf("ids = %s / %s", gotClient, gotTrainer)
	}
	if want := now.Add(clientAnalyticsLinkTTL).Unix(); exp != want {
		t.Fatalf("exp = %d, want %d", exp, want)
	}
	if !VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, sig, now) {
		t.Fatal("fresh link must verify")
	}
	if !VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, sig, now.Add(29*time.Minute)) {
		t.Fatal("link must verify before expiry")
	}
}

func TestClientAnalyticsLink_expired(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	clientID, trainerID := uuid.New(), uuid.New()
	link := BuildClientAnalyticsLink("https://app.example.com/stats", testLinkSecret, clientID, trainerID, now)
	_, _, exp, sig := parseLink(t, link)

	if VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, sig, now.Add(31*time.Minute)) {
		t.Fatal("expired link must not verify")
	}
}

func TestClientAnalyticsLink_tamper(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	clientID, trainerID := uuid.New(), uuid.New()
	link := BuildClientAnalyticsLink("https://app.example.com/stats", testLinkSecret, clientID, trainerID, now)
	_, _, exp, sig := parseLink(t, link)

	// Flip the last hex digit so the tampered signature always differs.
	flipped := sig[:len(sig)-1] + "0"
	if sig[len(sig)-1] == '0' {
		flipped = sig[:len(sig)-1] + "1"
	}

	cases := map[string]bool{
		"other client":  VerifyClientAnalyticsLink(testLinkSecret, uuid.New(), trainerID, exp, sig, now),
		"other trainer": VerifyClientAnalyticsLink(testLinkSecret, clientID, uuid.New(), exp, sig, now),
		"other exp":     VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp+1, sig, now),
		"other sig":     VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, flipped, now),
		"other secret":  VerifyClientAnalyticsLink("another-secret-at-least-32-chars-long!!", clientID, trainerID, exp, sig, now),
		"empty sig":     VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, "", now),
		"empty secret":  VerifyClientAnalyticsLink("", clientID, trainerID, exp, sig, now),
		"zero exp":      VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, 0, sig, now),
	}
	for name, ok := range cases {
		if ok {
			t.Errorf("%s: must not verify", name)
		}
	}
}

// An avatar signature over the same client id must not open the analytics page:
// the two link kinds live in different HMAC domains.
func TestClientAnalyticsLink_avatarSignatureRejected(t *testing.T) {
	clientID, trainerID := uuid.New(), uuid.New()
	avatar := trainerclient.BuildAvatarURL(clientID, "photos/x.jpg", testLinkSecret)
	u, err := url.Parse(avatar)
	if err != nil {
		t.Fatalf("parse avatar: %v", err)
	}
	exp, err := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	if err != nil {
		t.Fatalf("avatar exp: %v", err)
	}
	if VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, u.Query().Get("sig"), time.Now()) {
		t.Fatal("avatar signature must not verify as analytics link")
	}
}

func TestBuildClientAnalyticsLink_disabled(t *testing.T) {
	now := time.Now()
	if got := BuildClientAnalyticsLink("", testLinkSecret, uuid.New(), uuid.New(), now); got != "" {
		t.Fatalf("empty page url: got %q", got)
	}
	if got := BuildClientAnalyticsLink("https://app.example.com/stats", "", uuid.New(), uuid.New(), now); got != "" {
		t.Fatalf("empty secret: got %q", got)
	}
}

func TestClientLinkBuilder(t *testing.T) {
	b := NewClientLinkBuilder("https://app.example.com/stats", testLinkSecret)
	clientID, trainerID := uuid.New(), uuid.New()
	link := b.BuildClientAnalyticsLink(clientID, trainerID)
	gotClient, gotTrainer, exp, sig := parseLink(t, link)
	if gotClient != clientID || gotTrainer != trainerID {
		t.Fatalf("ids = %s / %s", gotClient, gotTrainer)
	}
	if !VerifyClientAnalyticsLink(testLinkSecret, clientID, trainerID, exp, sig, time.Now()) {
		t.Fatal("builder link must verify")
	}
}
