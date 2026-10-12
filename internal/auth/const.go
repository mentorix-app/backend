package auth

import "errors"

const (
	ProviderEmailPassword = "email_password"
	ProviderTelegram      = "telegram"
	ProviderGoogle        = "google"
	ProviderApple         = "apple"
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
	// ErrProviderNotConfigured means the sign-in provider has no client ids set, or,
	// for account deletion, that the Apple revocation is not configured.
	ErrProviderNotConfigured = errors.New("sign-in provider is not configured")
	// ErrInvalidLinkCode means the Telegram link code is empty, unknown, or expired.
	ErrInvalidLinkCode = errors.New("invalid or expired code")
	// ErrLinkCodesUnavailable means link codes cannot be stored, for example without Redis.
	ErrLinkCodesUnavailable = errors.New("telegram link codes are unavailable")
	// ErrTelegramLinkConflict means the Telegram belongs to an account that cannot be merged with this one.
	ErrTelegramLinkConflict = errors.New("telegram is linked to an account that cannot be merged")
	// ErrTelegramAlreadyLinked means the account already has a different Telegram.
	ErrTelegramAlreadyLinked = errors.New("account already has another telegram")
	// ErrAccountDeletionUnavailable means the app cannot delete this account: its roles are
	// not exactly client (no role at all included), or other rows still reference it.
	ErrAccountDeletionUnavailable = errors.New("this account cannot be deleted in the app")
	// ErrAppleSignInNotRevoked means the account has an Apple sign-in whose authorization
	// was not revoked at Apple, so the store refuses to delete it.
	ErrAppleSignInNotRevoked = errors.New("the account has an apple sign-in that was not revoked")
)
