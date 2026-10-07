package auth

import (
	"time"

	"mentorix-backend/internal/subscription"
)

// AuthCredentials is the JSON body for POST /auth/login.
// TokenDelivery is "cookie" (default) or "body".
type AuthCredentials struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	TokenDelivery string `json:"token_delivery,omitempty"`
}

// RegisterRequest is the JSON body for POST /auth/register.
// TokenDelivery is "cookie" (default) or "body".
type RegisterRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	Name          string `json:"name"`
	TokenDelivery string `json:"token_delivery,omitempty"`
}

// SocialLoginRequest is the JSON body for POST /auth/social-login.
// Provider is "apple" or "google". Name is optional and used only when the
// request creates an account. TokenDelivery is "cookie" (default) or "body".
type SocialLoginRequest struct {
	Provider      string `json:"provider"`
	IDToken       string `json:"id_token"`
	Name          string `json:"name,omitempty"`
	TokenDelivery string `json:"token_delivery,omitempty"`
}

// AddRoleRequest is the JSON body for POST /auth/me/roles.
// Role is "trainer" or "client".
type AddRoleRequest struct {
	Role string `json:"role"`
}

// RefreshRequest is the optional JSON body for POST /auth/refresh and POST /auth/logout.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// MePatchRequest is the JSON body for PATCH /auth/me.
type MePatchRequest struct {
	Name string `json:"name"`
}

// TokenResponse is the JSON body for successful register, login, social login, and refresh.
// Email is empty for accounts without a primary email.
// RefreshToken is set only when the client asked for body delivery.
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
}
