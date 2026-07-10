package config

import (
	"fmt"
	"net/http"
	"os"
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

	secure := appEnv == AppEnvProduction
	if v := strings.TrimSpace(os.Getenv("REFRESH_COOKIE_SECURE")); v == "true" {
		secure = true
	} else if v == "false" {
		secure = false
	}

	sameSite, err := sameSiteFromEnv(strings.TrimSpace(os.Getenv("REFRESH_COOKIE_SAMESITE")), appEnv)
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
		AuthLoginRateMax:     loginMax,
		AuthLoginRateWindow:  loginWin,
		AuthRegisterRateMax:  regMax,
		AuthRegisterRateWin:  regWin,
		TelegramBotUsername:  strings.TrimSpace(os.Getenv("TELEGRAM_BOT_USERNAME")),
		TrainerInviteTTLDays: inviteDays,
		BotToken:             strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		BotWebhookURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("BOT_WEBHOOK_URL")), "/"),
		BotWebhookSecret:     strings.TrimSpace(os.Getenv("BOT_WEBHOOK_SECRET")),
	}

	if cfg.AppEnv != AppEnvDevelopment && cfg.AppEnv != AppEnvProduction {
		return Config{}, fmt.Errorf("config: APP_ENV must be development or production, got %q", cfg.AppEnv)
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

	return cfg, nil
}

func sameSiteFromEnv(raw, appEnv string) (http.SameSite, error) {
	if raw == "" {
		if appEnv == AppEnvProduction {
			return http.SameSiteNoneMode, nil
		}
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
