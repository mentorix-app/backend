package auth

import (
	"time"

	"mentorix-backend/internal/subscription"
)

// MePatchRequest is the JSON body for PATCH /auth/me.
type MePatchRequest struct {
	Name string `json:"name"`
}

// IDTokenRequest is the JSON body for POST /auth/google.
type IDTokenRequest struct {
	IDToken string `json:"id_token"`
	// Nonce is optional: the raw value, at most 256 characters. When set, the ID
	// token's nonce claim must be its SHA-256 as lowercase hex.
	Nonce string `json:"nonce"`
}

// AppleSignInRequest is the JSON body for POST /auth/apple. Apple puts no name
// in the token, so the app sends the one it received on the first authorization.
type AppleSignInRequest struct {
	IDToken string `json:"id_token"`
	Name    string `json:"name"`
	// Nonce is required: the raw value, 1 to 256 characters. The ID token's nonce
	// claim must be its SHA-256 as lowercase hex.
	Nonce string `json:"nonce"`
}

// DeleteAccountRequest is the optional JSON body for DELETE /auth/me. A client whose
// account has a Sign in with Apple identity sends a fresh Apple ID token and the
// authorization code of the same sign-in so that the server can revoke the
// authorization before deleting. The redirect address goes along only when the
// authorization request had one (Android). Other accounts leave the body out.
type DeleteAccountRequest struct {
	AppleIDToken           string `json:"apple_id_token"`
	AppleAuthorizationCode string `json:"apple_authorization_code"`
	AppleRedirectURI       string `json:"apple_redirect_uri"`
}

// TrainerActivationRequest is the JSON body for POST /auth/me/trainer. TermsVersion is the
// version of the trainer terms the person accepted: a calendar date as YYYY-MM-DD.
type TrainerActivationRequest struct {
	TermsVersion string `json:"terms_version"`
}

// TelegramLinkRequest is the JSON body for POST /auth/telegram/link. Code is the
// one-time code the bot sent to the Telegram user.
type TelegramLinkRequest struct {
	Code string `json:"code"`
}

// RefreshRequest is the optional JSON body for POST /auth/refresh and POST /auth/logout.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// TokenResponse is the JSON body for successful refresh, Google and
// Apple sign-in, and Telegram link. RefreshExpiresAt is always set, also when
// the refresh token travels in a cookie. RefreshToken is set only for clients that
// keep the refresh token themselves (Google and Apple sign-in, Telegram link,
// body-mode refresh); cookie flows leave it out.
type TokenResponse struct {
	AccessToken      string    `json:"access_token"`
	TokenType        string    `json:"token_type"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	UserID           string    `json:"user_id"`
	Email            string    `json:"email"`
	RefreshToken     string    `json:"refresh_token,omitempty"`
}

// MeResponse is the JSON body for GET /auth/me and PATCH /auth/me.
// Subscription is set for users with a trainer profile and null otherwise.
type MeResponse struct {
	UserID       string                     `json:"user_id"`
	Email        string                     `json:"email"`
	Name         string                     `json:"name"`
	CreatedAt    time.Time                  `json:"created_at"`
	Roles        []string                   `json:"roles"`
	Subscription *subscription.Subscription `json:"subscription"`
	// TelegramLinked tells the app whether the account already has a Telegram sign-in.
	TelegramLinked bool `json:"telegram_linked"`
}
