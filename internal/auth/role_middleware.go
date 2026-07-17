package auth

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
	httpx "mentorix-backend/internal/http"
)

func roleMiddleware(q *sqlc.Queries, forbiddenMsg string, roles ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			uid, ok := UserIDFromContext(c)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
			}
			allowed, err := q.UserHasAnyRole(c.Request().Context(), sqlc.UserHasAnyRoleParams{
				UserID: pgconv.ToPGUUID(uid),
				Roles:  roles,
			})
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
			}
			if !allowed {
				return echo.NewHTTPError(http.StatusForbidden, forbiddenMsg)
			}
			return next(c)
		}
	}
}

func TrainerMiddleware(pool *pgxpool.Pool) echo.MiddlewareFunc {
	return roleMiddleware(sqlc.New(pool), httpx.MsgTrainerRoleRequired, RoleTrainer)
}

// TrainerOrAdminMiddleware guards read endpoints where admins have view-only access.
func TrainerOrAdminMiddleware(pool *pgxpool.Pool) echo.MiddlewareFunc {
	return roleMiddleware(sqlc.New(pool), httpx.MsgTrainerRoleRequired, RoleTrainer, RoleAdmin)
}

func AdminMiddleware(pool *pgxpool.Pool) echo.MiddlewareFunc {
	return roleMiddleware(sqlc.New(pool), httpx.MsgAdminRoleRequired, RoleAdmin)
}
