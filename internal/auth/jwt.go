package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const issuer = "mentorix-backend"

func signAccessToken(userID uuid.UUID, secret []byte, ttl time.Duration) (token string, expiresAt time.Time, err error) {
	now := time.Now().UTC()
	exp := now.Add(ttl)
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := t.SignedString(secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return s, exp, nil
}
