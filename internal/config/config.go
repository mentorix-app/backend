package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	AppEnv             string
	Port               string
	DatabaseURL        string
	RedisURL           string
	JWTSecret          string
	CORSAllowedOrigins []string
}

func Load() (Config, error) {
	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	if appEnv == "" {
		appEnv = "development"
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	cfg := Config{
		AppEnv:             appEnv,
		Port:               port,
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:           strings.TrimSpace(os.Getenv("REDIS_URL")),
		JWTSecret:          strings.TrimSpace(os.Getenv("JWT_SECRET")),
		CORSAllowedOrigins: parseCommaSeparated(os.Getenv("CORS_ALLOW_ORIGINS")),
	}

	if cfg.AppEnv != "development" && cfg.AppEnv != "production" {
		return Config{}, fmt.Errorf("config: APP_ENV must be development or production, got %q", cfg.AppEnv)
	}

	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("config: JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("config: JWT_SECRET must be at least 32 bytes for HS256")
	}

	return cfg, nil
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
