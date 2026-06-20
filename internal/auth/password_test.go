package auth

import (
	"strings"
	"testing"
)

func TestValidatePassword_bounds(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Error("expected error for short password")
	}
	long := strings.Repeat("a", maxPasswordLen+1)
	if err := ValidatePassword(long); err == nil {
		t.Error("expected error for long password")
	}
	if err := ValidatePassword(strings.Repeat("a", minPasswordLen)); err != nil {
		t.Errorf("valid length password rejected: %v", err)
	}
}

func TestHashPasswordAndMatch(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := PasswordMatches(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("PasswordMatches: %v", err)
	}
	if !ok {
		t.Error("expected password to match")
	}
	ok, err = PasswordMatches(hash, "wrong password")
	if err != nil {
		t.Fatalf("PasswordMatches wrong: %v", err)
	}
	if ok {
		t.Error("expected password mismatch")
	}
}

func TestNormalizeEmail(t *testing.T) {
	got := NormalizeEmail("  User@Example.COM ")
	if got != "user@example.com" {
		t.Errorf("NormalizeEmail = %q", got)
	}
}
