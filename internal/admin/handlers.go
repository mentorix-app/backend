package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
)

type Handlers struct {
	svc       *Service
	jwtSecret string
	pool      *pgxpool.Pool
}

func NewHandlers(svc *Service, pool *pgxpool.Pool, jwtSecret string) *Handlers {
	return &Handlers{svc: svc, pool: pool, jwtSecret: jwtSecret}
}

func (h *Handlers) Mount(e *echo.Echo) {
	g := e.Group("/admin",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerMiddleware(h.pool),
	)
	g.POST("/users/:user_id/roles/admin", h.GrantAdmin)
}

type userResponse struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	Roles     []string  `json:"roles"`
}

func (h *Handlers) GrantAdmin(c echo.Context) error {
	targetUserID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidUserID)
	}
	profile, err := h.svc.GrantAdmin(c.Request().Context(), targetUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgUserNotFound)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "grant admin failed")
	}
	return c.JSON(http.StatusOK, userResponse{
		UserID:    targetUserID.String(),
		Email:     profile.Email,
		CreatedAt: profile.CreatedAt,
		Roles:     profile.Roles,
	})
}
