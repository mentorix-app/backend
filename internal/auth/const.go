package auth

import "errors"

const (
	ProviderEmailPassword = "email_password"
	ProviderTelegram      = "telegram"
	ProviderGoogle        = "google"
)

const (
	RoleAdmin   = "admin"
	RoleTrainer = "trainer"
	RoleClient  = "client"
)

const (
	TokenTypeBearer  = "Bearer"
	AuthSchemeBearer = "Bearer"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
	ErrInvalidIDToken     = errors.New("invalid id token")
	// ErrProviderNotConfigured means the sign-in provider has no client ids set.
	ErrProviderNotConfigured = errors.New("sign-in provider is not configured")
)
