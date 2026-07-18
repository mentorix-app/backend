package admin

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/subscription"
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
		auth.AdminMiddleware(h.pool),
	)
	g.PUT("/trainers/:user_id/plan", h.GrantPlan)
	g.DELETE("/trainers/:user_id/plan", h.RevokePlan)
}

type grantPlanBody struct {
	Plan subscription.Plan `json:"plan"`
}

type planResponse struct {
	UserID       string                     `json:"user_id"`
	Subscription *subscription.Subscription `json:"subscription"`
}

func (h *Handlers) GrantPlan(c echo.Context) error {
	targetUserID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidUserID)
	}
	var body grantPlanBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	sub, err := h.svc.GrantPlan(c.Request().Context(), targetUserID, body.Plan)
	if err != nil {
		return planHTTPError(err)
	}
	return c.JSON(http.StatusOK, planResponse{UserID: targetUserID.String(), Subscription: sub})
}

func (h *Handlers) RevokePlan(c echo.Context) error {
	targetUserID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidUserID)
	}
	sub, err := h.svc.RevokePlan(c.Request().Context(), targetUserID)
	if err != nil {
		return planHTTPError(err)
	}
	return c.JSON(http.StatusOK, planResponse{UserID: targetUserID.String(), Subscription: sub})
}

func planHTTPError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, subscription.ErrInvalidPlan):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, subscription.ErrTrainerNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "plan operation failed")
	}
}
