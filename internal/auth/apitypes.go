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

// TokenResponse is the JSON body for successful register, login, and refresh.
type TokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
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
}
