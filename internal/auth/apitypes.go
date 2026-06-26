package auth

import "time"

// AuthCredentials is the JSON body for POST /auth/register and POST /auth/login.
type AuthCredentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TokenResponse is the JSON body for successful register, login, and refresh.
type TokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
}

// MeResponse is the JSON body for GET /auth/me.
type MeResponse struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	Roles     []string  `json:"roles"`
}
