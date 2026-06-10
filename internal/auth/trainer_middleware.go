package auth

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func TrainerMiddleware(pool *pgxpool.Pool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			uid, ok := UserIDFromContext(c)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
			}
			var allowed bool
			err := pool.QueryRow(c.Request().Context(),
				`SELECT EXISTS(
					SELECT 1 FROM mentorix.user_roles
					WHERE user_id = $1 AND role IN ('trainer', 'admin')
				)`,
				uid,
			).Scan(&allowed)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "internal")
			}
			if !allowed {
				return echo.NewHTTPError(http.StatusForbidden, "trainer role required")
			}
			return next(c)
		}
	}
}
