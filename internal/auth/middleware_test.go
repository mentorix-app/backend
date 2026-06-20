package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const testJWTSecret = "test-jwt-secret-at-least-32-characters-long"

func TestJWTMiddleware_missingAuthorization(t *testing.T) {
	e := echo.New()
	e.GET("/", JWTMiddleware(testJWTSecret)(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestJWTMiddleware_invalidBearerFormat(t *testing.T) {
	e := echo.New()
	e.GET("/", JWTMiddleware(testJWTSecret)(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Token abc")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestJWTMiddleware_validToken(t *testing.T) {
	userID := uuid.New()
	token, _, err := signAccessToken(userID, []byte(testJWTSecret), time.Minute)
	if err != nil {
		t.Fatalf("signAccessToken: %v", err)
	}

	var got uuid.UUID
	e := echo.New()
	e.GET("/", JWTMiddleware(testJWTSecret)(func(c echo.Context) error {
		id, ok := UserIDFromContext(c)
		if !ok {
			t.Fatal("expected user id in context")
		}
		got = id
		return c.NoContent(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got != userID {
		t.Fatalf("user id = %v, want %v", got, userID)
	}
}

func TestJWTMiddleware_wrongIssuer(t *testing.T) {
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Issuer:    "other-service",
		Subject:   uuid.New().String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	e := echo.New()
	e.GET("/", JWTMiddleware(testJWTSecret)(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestJWTMiddleware_expiredToken(t *testing.T) {
	userID := uuid.New()
	token, _, err := signAccessToken(userID, []byte(testJWTSecret), -time.Minute)
	if err != nil {
		t.Fatalf("signAccessToken: %v", err)
	}

	e := echo.New()
	e.GET("/", JWTMiddleware(testJWTSecret)(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
