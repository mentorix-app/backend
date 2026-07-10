package config

import (
	"net/http"
	"testing"
	"time"
)

func setMinimalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-jwt-secret-at-least-32-chars-long")
	t.Setenv("APP_ENV", "development")
}

func TestLoad_requiresJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty JWT_SECRET")
	}
}

func TestLoad_jwtSecretMinLength(t *testing.T) {
	t.Setenv("JWT_SECRET", "too-short")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for short JWT_SECRET")
	}
}

func TestLoad_defaults(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("PORT", "")
	t.Setenv("ACCESS_TTL_MINUTES", "")
	t.Setenv("REFRESH_TTL_DAYS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.AccessTTLMinutes != 15 {
		t.Errorf("AccessTTLMinutes = %d, want 15", cfg.AccessTTLMinutes)
	}
	if cfg.RefreshTTLDays != 30 {
		t.Errorf("RefreshTTLDays = %d, want 30", cfg.RefreshTTLDays)
	}
	if cfg.AccessTokenTTL() != 15*time.Minute {
		t.Errorf("AccessTokenTTL = %v", cfg.AccessTokenTTL())
	}
	if cfg.RefreshTokenTTL() != 30*24*time.Hour {
		t.Errorf("RefreshTokenTTL = %v", cfg.RefreshTokenTTL())
	}
}

func TestLoad_refreshCookie(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("REFRESH_COOKIE_NAME", "custom_refresh")
	t.Setenv("REFRESH_COOKIE_PATH", "/api/auth")
	t.Setenv("REFRESH_COOKIE_SAMESITE", "strict")
	t.Setenv("REFRESH_COOKIE_SECURE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshCookie.Name != "custom_refresh" {
		t.Errorf("cookie name = %q", cfg.RefreshCookie.Name)
	}
	if cfg.RefreshCookie.Path != "/api/auth" {
		t.Errorf("cookie path = %q", cfg.RefreshCookie.Path)
	}
	if cfg.RefreshCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", cfg.RefreshCookie.SameSite)
	}
	if !cfg.RefreshCookie.Secure {
		t.Error("expected Secure=true")
	}
}

func TestLoad_invalidSameSite(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("REFRESH_COOKIE_SAMESITE", "invalid")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid REFRESH_COOKIE_SAMESITE")
	}
}

func TestLoad_rateLimits(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("AUTH_LOGIN_RATE_MAX", "5")
	t.Setenv("AUTH_LOGIN_RATE_WINDOW_SEC", "60")
	t.Setenv("AUTH_REGISTER_RATE_MAX", "3")
	t.Setenv("AUTH_REGISTER_RATE_WINDOW_SEC", "120")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuthLoginRateMax != 5 || cfg.AuthLoginRateWindow != 60*time.Second {
		t.Errorf("login rate: max=%d window=%v", cfg.AuthLoginRateMax, cfg.AuthLoginRateWindow)
	}
	if cfg.AuthRegisterRateMax != 3 || cfg.AuthRegisterRateWin != 120*time.Second {
		t.Errorf("register rate: max=%d window=%v", cfg.AuthRegisterRateMax, cfg.AuthRegisterRateWin)
	}
}

func TestLoad_corsOrigins(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CORS_ALLOW_ORIGINS", " http://localhost:3000 , https://app.example.com ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("origins = %v", cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("first origin = %q", cfg.CORSAllowedOrigins[0])
	}
}

func TestLoad_invalidAppEnv(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("APP_ENV", "staging")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid APP_ENV")
	}
}

func TestLoad_sameSiteDefaults(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("REFRESH_COOKIE_SAMESITE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("dev default SameSite = %v, want Lax", cfg.RefreshCookie.SameSite)
	}

	t.Setenv("APP_ENV", "production")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load production: %v", err)
	}
	if cfg.RefreshCookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("prod default SameSite = %v, want None", cfg.RefreshCookie.SameSite)
	}
}

func TestLoad_sameSiteLaxAndNone(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("REFRESH_COOKIE_SAMESITE", "lax")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load lax: %v", err)
	}
	if cfg.RefreshCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v", cfg.RefreshCookie.SameSite)
	}

	t.Setenv("REFRESH_COOKIE_SAMESITE", "none")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load none: %v", err)
	}
	if cfg.RefreshCookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("SameSite = %v", cfg.RefreshCookie.SameSite)
	}
}

func TestLoad_trainerInviteSettings(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("TRAINER_INVITE_TTL_DAYS", "14")
	t.Setenv("TELEGRAM_BOT_USERNAME", "mentorix_bot")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TrainerInviteTTLDays != 14 {
		t.Errorf("TrainerInviteTTLDays = %d, want 14", cfg.TrainerInviteTTLDays)
	}
	if cfg.TrainerInviteTTL() != 14*24*time.Hour {
		t.Errorf("TrainerInviteTTL = %v", cfg.TrainerInviteTTL())
	}
	if cfg.TelegramBotUsername != "mentorix_bot" {
		t.Errorf("TelegramBotUsername = %q", cfg.TelegramBotUsername)
	}
}

func TestLoad_trainerInviteTTLDefaultWhenInvalid(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("TRAINER_INVITE_TTL_DAYS", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TrainerInviteTTLDays != 7 {
		t.Errorf("TrainerInviteTTLDays = %d, want 7", cfg.TrainerInviteTTLDays)
	}
}

func TestLoad_botTokenOptional(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("BOT_TOKEN", " 123:abc ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BotToken != "123:abc" {
		t.Errorf("BotToken = %q, want 123:abc", cfg.BotToken)
	}
}

func TestLoad_webhookRequiresTokenAndSecret(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("BOT_WEBHOOK_URL", "https://api.example.com/telegram/webhook")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error without BOT_TOKEN")
	}

	t.Setenv("BOT_TOKEN", "123:abc")
	_, err = Load()
	if err == nil {
		t.Fatal("expected error without BOT_WEBHOOK_SECRET")
	}
}
