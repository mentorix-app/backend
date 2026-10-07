package auth

import "errors"

const (
	ProviderEmailPassword = "email_password"
	ProviderTelegram      = "telegram"
	ProviderApple         = "apple"
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

const (
	TokenDeliveryCookie = "cookie"
	TokenDeliveryBody   = "body"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")

	ErrProviderNotConfigured           = errors.New("provider not configured")
	ErrEmailBelongsToAnotherAccount    = errors.New("email belongs to another account")
	ErrRoleConflict                    = errors.New("role conflicts with existing roles")
	ErrIdentityBelongsToAnotherAccount = errors.New("identity belongs to another account")

	ErrPasswordRequired          = errors.New("current password is required")
	ErrPasswordIncorrect         = errors.New("current password is incorrect")
	ErrAdminCannotAttachIdentity = errors.New("admins cannot add sign-in methods")
)
