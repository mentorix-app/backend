package auth

import "errors"

const ProviderEmailPassword = "email_password"

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
)
