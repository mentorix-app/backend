package analytics

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// clientAnalyticsLinkTTL bounds how long a link from the Telegram bot opens
// the client's analytics page. The link is a bearer secret: keep it short.
const clientAnalyticsLinkTTL = 30 * time.Minute

// clientAnalyticsLinkDomain separates these signatures from avatar URLs that
// share the same secret.
const clientAnalyticsLinkDomain = "client_analytics:"

// BuildClientAnalyticsLink returns the frontend page URL with a signed query
// string, or "" when the feature is not configured.
func BuildClientAnalyticsLink(pageURL, secret string, clientUserID, trainerID uuid.UUID, now time.Time) string {
	if pageURL == "" || secret == "" {
		return ""
	}
	exp := now.Add(clientAnalyticsLinkTTL).Unix()
	sig := signClientAnalyticsLink(secret, clientUserID, trainerID, exp)
	return fmt.Sprintf("%s?client_user_id=%s&trainer_id=%s&exp=%d&sig=%s", pageURL, clientUserID, trainerID, exp, sig)
}

// VerifyClientAnalyticsLink checks the signature and expiry of query
// parameters forwarded by the frontend.
func VerifyClientAnalyticsLink(secret string, clientUserID, trainerID uuid.UUID, exp int64, sig string, now time.Time) bool {
	if secret == "" || sig == "" || exp <= 0 {
		return false
	}
	if now.Unix() > exp {
		return false
	}
	expected := signClientAnalyticsLink(secret, clientUserID, trainerID, exp)
	return hmac.Equal([]byte(expected), []byte(sig))
}

func signClientAnalyticsLink(secret string, clientUserID, trainerID uuid.UUID, exp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(clientAnalyticsLinkDomain))
	_, _ = mac.Write([]byte(clientUserID.String()))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(trainerID.String()))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// ClientLinkBuilder is what the Telegram bot uses to issue links; it hides the
// page URL and secret from the bot package.
type ClientLinkBuilder struct {
	pageURL string
	secret  string
}

func NewClientLinkBuilder(pageURL, secret string) *ClientLinkBuilder {
	return &ClientLinkBuilder{pageURL: pageURL, secret: secret}
}

func (b *ClientLinkBuilder) BuildClientAnalyticsLink(clientUserID, trainerID uuid.UUID) string {
	return BuildClientAnalyticsLink(b.pageURL, b.secret, clientUserID, trainerID, time.Now())
}
