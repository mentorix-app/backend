package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RefreshCookieSettings struct {
	Name     string
	Path     string
	Domain   string
	Secure   bool
	SameSite http.SameSite
}

type Config struct {
	AppEnv               string
	Port                 string
	DatabaseURL          string
	RedisURL             string
	JWTSecret            string
	CORSAllowedOrigins   []string
	TrustedProxyCIDRs    []string
	AccessTTLMinutes     int
	RefreshTTLDays       int
	RefreshCookie        RefreshCookieSettings
	AuthLoginRateMax     int
	AuthLoginRateWindow  time.Duration
	AuthRegisterRateMax  int
	AuthRegisterRateWin  time.Duration
	TelegramBotUsername  string
	TrainerInviteTTLDays int
	BotToken             string
	BotWebhookURL        string
	BotWebhookSecret     string
	// GoogleClientIDs are the OAuth client ids whose ID tokens POST /auth/google
	// accepts. Empty disables Google sign-in.
	GoogleClientIDs []string
	// AppleClientIDs are the audiences POST /auth/apple accepts: the iOS bundle
	// id and, for Android, the Services ID. Empty disables Apple sign-in.
	AppleClientIDs []string
	// AppleAndroidPackage is the Android application id that POST
	// /auth/apple/callback sends the browser back to. Empty disables the
	// callback.
	AppleAndroidPackage string
	// AppleTeamID, AppleKeyID and ApplePrivateKey sign the client secret that
	// DELETE /auth/me uses to revoke a Sign in with Apple authorization. All three
	// are set or none is; none disables the revocation.
	AppleTeamID     string
	AppleKeyID      string
	ApplePrivateKey *ecdsa.PrivateKey
	// ClientAnalyticsPageURL is the frontend page the Telegram bot links to for
	// client self-analytics. Empty disables the "Статистика" button.
	ClientAnalyticsPageURL string
	// GitCommit is the commit the binary was deployed from (RENDER_GIT_COMMIT on
	// Render). Empty locally; reported by GET /health when set.
	GitCommit string
}

func (c Config) AccessTokenTTL() time.Duration {
	return time.Duration(c.AccessTTLMinutes) * time.Minute
}

func (c Config) TrainerInviteTTL() time.Duration {
	return time.Duration(c.TrainerInviteTTLDays) * 24 * time.Hour
}

func (c Config) RefreshTokenTTL() time.Duration {
	return time.Duration(c.RefreshTTLDays) * 24 * time.Hour
}

func Load() (Config, error) {
	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	if appEnv == "" {
		appEnv = AppEnvDevelopment
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	accessMin := intFromEnv("ACCESS_TTL_MINUTES", 15)
	if accessMin < 1 {
		accessMin = 15
	}
	refreshDays := intFromEnv("REFRESH_TTL_DAYS", 30)
	if refreshDays < 1 {
		refreshDays = 30
	}

	cookieName := strings.TrimSpace(os.Getenv("REFRESH_COOKIE_NAME"))
	if cookieName == "" {
		cookieName = "mentorix_refresh"
	}
	cookiePath := strings.TrimSpace(os.Getenv("REFRESH_COOKIE_PATH"))
	if cookiePath == "" {
		cookiePath = "/auth"
	}
	cookieDomain := strings.TrimSpace(os.Getenv("REFRESH_COOKIE_DOMAIN"))

	secure := strings.TrimSpace(os.Getenv("REFRESH_COOKIE_SECURE")) == "true"

	sameSite, err := sameSiteFromEnv(strings.TrimSpace(os.Getenv("REFRESH_COOKIE_SAMESITE")))
	if err != nil {
		return Config{}, err
	}

	loginMax := intFromEnv("AUTH_LOGIN_RATE_MAX", 20)
	loginWin := secondsFromEnv("AUTH_LOGIN_RATE_WINDOW_SEC", 900)
	regMax := intFromEnv("AUTH_REGISTER_RATE_MAX", 10)
	regWin := secondsFromEnv("AUTH_REGISTER_RATE_WINDOW_SEC", 900)
	inviteDays := intFromEnv("TRAINER_INVITE_TTL_DAYS", 7)
	if inviteDays < 1 {
		inviteDays = 7
	}

	cfg := Config{
		AppEnv:             appEnv,
		Port:               port,
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:           strings.TrimSpace(os.Getenv("REDIS_URL")),
		JWTSecret:          strings.TrimSpace(os.Getenv("JWT_SECRET")),
		CORSAllowedOrigins: parseCommaSeparated(os.Getenv("CORS_ALLOW_ORIGINS")),
		TrustedProxyCIDRs:  parseCommaSeparated(os.Getenv("TRUSTED_PROXY_CIDRS")),
		AccessTTLMinutes:   accessMin,
		RefreshTTLDays:     refreshDays,
		RefreshCookie: RefreshCookieSettings{
			Name:     cookieName,
			Path:     cookiePath,
			Domain:   cookieDomain,
			Secure:   secure,
			SameSite: sameSite,
		},
		AuthLoginRateMax:       loginMax,
		AuthLoginRateWindow:    loginWin,
		AuthRegisterRateMax:    regMax,
		AuthRegisterRateWin:    regWin,
		TelegramBotUsername:    strings.TrimSpace(os.Getenv("TELEGRAM_BOT_USERNAME")),
		TrainerInviteTTLDays:   inviteDays,
		BotToken:               strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		BotWebhookURL:          strings.TrimRight(strings.TrimSpace(os.Getenv("BOT_WEBHOOK_URL")), "/"),
		BotWebhookSecret:       strings.TrimSpace(os.Getenv("BOT_WEBHOOK_SECRET")),
		GoogleClientIDs:        parseCommaSeparated(os.Getenv("GOOGLE_CLIENT_IDS")),
		AppleClientIDs:         parseCommaSeparated(os.Getenv("APPLE_CLIENT_IDS")),
		AppleAndroidPackage:    strings.TrimSpace(os.Getenv("APPLE_ANDROID_PACKAGE")),
		ClientAnalyticsPageURL: strings.TrimSpace(os.Getenv("CLIENT_ANALYTICS_PAGE_URL")),
		GitCommit:              strings.TrimSpace(os.Getenv("RENDER_GIT_COMMIT")),
		AppleTeamID:            strings.TrimSpace(os.Getenv("APPLE_TEAM_ID")),
		AppleKeyID:             strings.TrimSpace(os.Getenv("APPLE_KEY_ID")),
	}

	if cfg.AppEnv != AppEnvDevelopment {
		return Config{}, fmt.Errorf("config: APP_ENV must be development, got %q", cfg.AppEnv)
	}

	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("config: JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("config: JWT_SECRET must be at least 32 bytes for HS256")
	}

	if cfg.BotWebhookURL != "" && cfg.BotToken == "" {
		return Config{}, fmt.Errorf("config: BOT_TOKEN is required when BOT_WEBHOOK_URL is set")
	}
	if cfg.BotWebhookURL != "" && cfg.BotWebhookSecret == "" {
		return Config{}, fmt.Errorf("config: BOT_WEBHOOK_SECRET is required when BOT_WEBHOOK_URL is set")
	}

	if cfg.ClientAnalyticsPageURL != "" {
		if err := validatePageURL(cfg.ClientAnalyticsPageURL); err != nil {
			return Config{}, fmt.Errorf("config: CLIENT_ANALYTICS_PAGE_URL %w", err)
		}
	}

	if cfg.AppleAndroidPackage != "" {
		if err := validateAndroidPackage(cfg.AppleAndroidPackage); err != nil {
			return Config{}, fmt.Errorf("config: APPLE_ANDROID_PACKAGE %w", err)
		}
	}

	rawAppleKey := strings.TrimSpace(os.Getenv("APPLE_PRIVATE_KEY"))
	if cfg.AppleTeamID != "" || cfg.AppleKeyID != "" || rawAppleKey != "" {
		key, err := validateAppleRevocation(cfg.AppleTeamID, cfg.AppleKeyID, rawAppleKey)
		if err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
		cfg.ApplePrivateKey = key
	}

	return cfg, nil
}

func sameSiteFromEnv(raw string) (http.SameSite, error) {
	if raw == "" {
		return http.SameSiteLaxMode, nil
	}
	switch strings.ToLower(raw) {
	case SameSiteLax:
		return http.SameSiteLaxMode, nil
	case SameSiteStrict:
		return http.SameSiteStrictMode, nil
	case SameSiteNone:
		return http.SameSiteNoneMode, nil
	default:
		return http.SameSiteDefaultMode, fmt.Errorf("config: REFRESH_COOKIE_SAMESITE must be lax, strict, or none, got %q", raw)
	}
}

func intFromEnv(key string, defaultVal int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

func secondsFromEnv(key string, defaultSec int) time.Duration {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return time.Duration(defaultSec) * time.Second
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return time.Duration(defaultSec) * time.Second
	}
	return time.Duration(v) * time.Second
}

func parseCommaSeparated(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validatePageURL accepts only an absolute http(s) URL without query and
// fragment: the bot appends its own query string to it.
func validatePageURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("must use http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("must be absolute")
	}
	if strings.ContainsAny(raw, "?#") {
		return fmt.Errorf("must not contain query or fragment")
	}
	return nil
}

var androidPackagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

// validateAndroidPackage accepts only a dotted Android application id. The value
// goes into a redirect link, so characters such as ';' and '#' must not pass.
func validateAndroidPackage(raw string) error {
	if !androidPackagePattern.MatchString(raw) {
		return fmt.Errorf("must be an Android package name such as com.example.app, got %q", raw)
	}
	return nil
}

var appleIDPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// validateAppleRevocation checks the three settings that sign the Apple client secret
// and returns the parsed key. It never puts the key, or a fragment of it, into an
// error: the messages are fixed texts.
func validateAppleRevocation(teamID, keyID, rawKey string) (*ecdsa.PrivateKey, error) {
	if teamID == "" || keyID == "" || rawKey == "" {
		return nil, errors.New("APPLE_TEAM_ID, APPLE_KEY_ID and APPLE_PRIVATE_KEY must be set together or all left empty")
	}
	if !appleIDPattern.MatchString(teamID) {
		return nil, errors.New("APPLE_TEAM_ID must be 10 characters, A-Z and 0-9")
	}
	if !appleIDPattern.MatchString(keyID) {
		return nil, errors.New("APPLE_KEY_ID must be 10 characters, A-Z and 0-9")
	}
	// A one-line value (a Dashboard field) carries the line breaks as a backslash and an n.
	block, _ := pem.Decode([]byte(strings.ReplaceAll(rawKey, `\n`, "\n")))
	if block == nil {
		return nil, errors.New("APPLE_PRIVATE_KEY must be the PEM text of the .p8 file")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("APPLE_PRIVATE_KEY must be a PKCS#8 private key")
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("APPLE_PRIVATE_KEY must be an ECDSA key on curve P-256")
	}
	return key, nil
}
