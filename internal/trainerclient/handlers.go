package trainerclient

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/program"
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
	trainer := e.Group("/trainer",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerMiddleware(h.pool),
	)
	trainer.POST("/invites", h.CreateInvite)

	clients := e.Group("/trainer/clients",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerMiddleware(h.pool),
	)
	clients.GET("", h.ListClients)
	clients.GET("/:client_user_id/program-assignment", h.GetProgramAssignment)
	clients.PUT("/:client_user_id/program-assignment", h.SetProgramAssignment)
}

func (h *Handlers) CreateInvite(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	invite, err := h.svc.CreateInvite(c.Request().Context(), trainerUserID)
	if err != nil {
		return HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusCreated, invite)
}

func (h *Handlers) ListClients(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	params, err := ParseListParams(
		c.QueryParam("page"),
		c.QueryParam("limit"),
		c.QueryParam("sort_by"),
		c.QueryParam("sort_order"),
		c.QueryParam("q"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	result, err := h.svc.ListClients(c.Request().Context(), trainerUserID, params)
	if err != nil {
		return HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) GetProgramAssignment(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	clientUserID, err := uuid.Parse(c.Param("client_user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}

	assignment, err := h.svc.GetClientProgramAssignment(c.Request().Context(), trainerUserID, clientUserID)
	if err != nil {
		return program.HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, assignment)
}

func (h *Handlers) SetProgramAssignment(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	clientUserID, err := uuid.Parse(c.Param("client_user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}

	var body program.SetClientProgramAssignmentRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}

	assignment, err := h.svc.SetClientProgramAssignment(c.Request().Context(), trainerUserID, clientUserID, body.ProgramID)
	if err != nil {
		return program.HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, assignment)
}
