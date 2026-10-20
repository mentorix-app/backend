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
	// ErrAdminCannotActivate means the account is an admin, and an admin cannot hold the
	// trainer role.
	ErrAdminCannotActivate = errors.New("an admin cannot become a trainer")
	// ErrAccountDeletionUnavailable means the app cannot delete this account: it has a role
	// besides client and trainer or none at all, its trainer side holds business data, or
	// other rows still reference it.
	ErrAccountDeletionUnavailable = errors.New("this account cannot be deleted in the app")
	// ErrAppleSignInNotRevoked means the account has an Apple sign-in whose authorization
	// was not revoked at Apple, so the store refuses to delete it.
	ErrAppleSignInNotRevoked = errors.New("the account has an apple sign-in that was not revoked")
	// ErrAppleAuthorizationRequired means the account has an Apple sign-in and the request
	// carries no Apple ID token and authorization code to revoke it with.
	ErrAppleAuthorizationRequired = errors.New("apple authorization is required")
	// ErrAppleAccountMismatch means the Apple account in play is not the right one: the
	// verified token belongs to an Apple account that is not one of the account's
	// sign-ins, or the authorization code was issued to an Apple account other than the
	// token's.
	ErrAppleAccountMismatch = errors.New("this apple account does not belong to the account")
	// ErrInvalidAppleAuthorizationCode means Apple refused the authorization code
	// (invalid_grant): it is wrong, expired, already used, or issued to another client.
	ErrInvalidAppleAuthorizationCode = errors.New("invalid or expired apple authorization code")
	// ErrAppleRevocationFailed means Apple did not confirm the revocation: any other
	// refusal, an answer that cannot be read, or no answer.
	ErrAppleRevocationFailed = errors.New("apple did not confirm the revocation")
)
