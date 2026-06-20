package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
)

const ContextUserIDKey = "user_id"

func JWTMiddleware(secret string) echo.MiddlewareFunc {
	key := []byte(secret)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Request().Header.Get(echo.HeaderAuthorization)
			if h == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgMissingAuth)
			}
			parts := strings.SplitN(h, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], AuthSchemeBearer) {
				return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgMissingBearerToken)
			}
			raw := strings.TrimSpace(parts[1])
			claims := &jwt.RegisteredClaims{}
			_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method")
				}
				return key, nil
			}, jwt.WithIssuer(Issuer))
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgInvalidToken)
			}
			id, err := uuid.Parse(claims.Subject)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgInvalidToken)
			}
			c.Set(ContextUserIDKey, id)
			return next(c)
		}
	}
}

func UserIDFromContext(c echo.Context) (uuid.UUID, bool) {
	v := c.Get(ContextUserIDKey)
	if v == nil {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}
