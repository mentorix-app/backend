package auth

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
)

const minPasswordLen = 8
const maxPasswordLen = 72

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < minPasswordLen {
		return fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}
	if n > maxPasswordLen {
		return fmt.Errorf("password must be at most %d characters", maxPasswordLen)
	}
	return nil
}

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func PasswordMatches(hash, password string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, hash)
}
