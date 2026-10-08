package auth

import (
	"time"

	"mentorix-backend/internal/subscription"
)

// AuthCredentials is the JSON body for POST /auth/login.
type AuthCredentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest is the JSON body for POST /auth/register.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// MePatchRequest is the JSON body for PATCH /auth/me.
type MePatchRequest struct {
	Name string `json:"name"`
}

// IDTokenRequest is the JSON body for POST /auth/google.
type IDTokenRequest struct {
	IDToken string `json:"id_token"`
}

// AppleSignInRequest is the JSON body for POST /auth/apple. Apple puts no name
// in the token, so the app sends the one it received on the first authorization.
type AppleSignInRequest struct {
	IDToken string `json:"id_token"`
	Name    string `json:"name"`
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

// TokenResponse is the JSON body for successful register, login, refresh, and Google sign-in.
// RefreshToken is set only for clients that keep the refresh token themselves
// (Google sign-in, body-mode refresh); cookie flows leave it out.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	RefreshToken string    `json:"refresh_token,omitempty"`
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
