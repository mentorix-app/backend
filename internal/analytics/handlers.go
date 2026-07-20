package analytics

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
)

type Handlers struct {
	svc       *Service
	pool      *pgxpool.Pool
	jwtSecret string
}

func NewHandlers(svc *Service, pool *pgxpool.Pool, jwtSecret string) *Handlers {
	return &Handlers{svc: svc, pool: pool, jwtSecret: jwtSecret}
}

func (h *Handlers) Mount(e *echo.Echo) {
	g := e.Group("/trainer",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerMiddleware(h.pool),
	)
	g.GET("/clients/:client_user_id/analytics", h.GetClientAnalytics)
	g.GET("/clients/:client_user_id/completions", h.ListClientCompletions)
	g.GET("/programs/analytics", h.ListProgramsAnalytics)
	g.GET("/programs/:program_id/analytics", h.GetProgramAnalytics)
}

func (h *Handlers) GetClientAnalytics(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	clientUserID, err := uuid.Parse(c.Param("client_user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}

	result, err := h.svc.ClientAnalytics(c.Request().Context(), trainerUserID, clientUserID)
	if err != nil {
		return HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) ListClientCompletions(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	clientUserID, err := uuid.Parse(c.Param("client_user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	params, err := ParseCompletionsParams(
		c.QueryParam("page"),
		c.QueryParam("limit"),
		c.QueryParam("from"),
		c.QueryParam("to"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := h.svc.ClientCompletions(c.Request().Context(), trainerUserID, clientUserID, params)
	if err != nil {
		return HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) ListProgramsAnalytics(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	params, err := ParseProgramsParams(
		c.QueryParam("page"),
		c.QueryParam("limit"),
		c.QueryParam("sort_by"),
		c.QueryParam("sort_order"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := h.svc.ProgramsAnalytics(c.Request().Context(), trainerUserID, params)
	if err != nil {
		return HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) GetProgramAnalytics(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := uuid.Parse(c.Param("program_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}

	result, err := h.svc.ProgramAnalytics(c.Request().Context(), trainerUserID, programID)
	if err != nil {
		return HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, result)
}

func HTTPErrorFrom(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
	case errors.Is(err, ErrClientNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "client not found")
	case errors.Is(err, ErrProgramNotFound):
		return echo.NewHTTPError(http.StatusNotFound, httpx.MsgProgramNotFound)
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
}
