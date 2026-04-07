package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	AppEnv       string
	Port         string
	DatabaseURL  string
	RedisURL     string
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
		AppEnv:      appEnv,
		Port:        port,
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:    strings.TrimSpace(os.Getenv("REDIS_URL")),
	}

	if cfg.AppEnv != "development" && cfg.AppEnv != "production" {
		return Config{}, fmt.Errorf("config: APP_ENV must be development or production, got %q", cfg.AppEnv)
	}

	return cfg, nil
}
