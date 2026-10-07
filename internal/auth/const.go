package auth

import (
	"errors"
	"fmt"
	"time"
)

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

// DefaultRefreshReuseGrace is how long a rotated refresh token can still be
// retried, for a client that lost the response to its first call.
const DefaultRefreshReuseGrace = 30 * time.Second

// MaxRefreshReuseGrace bounds WithRefreshReuseGrace. A longer window would leave a
// stolen spent token usable for longer.
const MaxRefreshReuseGrace = 5 * time.Minute

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
	// ErrRefreshBusy means another request holds the sign-in's lock for too long.
	// The client can retry the call.
	ErrRefreshBusy = errors.New("refresh session busy, try again")
	// ErrRefreshTokenReused means a rotated token came back after the grace window.
	// It wraps ErrInvalidRefresh so callers that only check for an invalid token still match.
	ErrRefreshTokenReused = fmt.Errorf("refresh token reused: %w", ErrInvalidRefresh)

	ErrProviderNotConfigured           = errors.New("provider not configured")
	ErrEmailBelongsToAnotherAccount    = errors.New("email belongs to another account")
	ErrRoleConflict                    = errors.New("role conflicts with existing roles")
	ErrIdentityBelongsToAnotherAccount = errors.New("identity belongs to another account")

	ErrPasswordRequired          = errors.New("current password is required")
	ErrPasswordIncorrect         = errors.New("current password is incorrect")
	ErrAdminCannotAttachIdentity = errors.New("admins cannot add sign-in methods")
)
