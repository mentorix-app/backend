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
	programs  ClientProgramService
	pool      *pgxpool.Pool
	jwtSecret string
}

func NewHandlers(programs ClientProgramService, pool *pgxpool.Pool, jwtSecret string) *Handlers {
	return &Handlers{programs: programs, pool: pool, jwtSecret: jwtSecret}
}

func (h *Handlers) Mount(e *echo.Echo) {
	g := e.Group("/trainer/clients",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerMiddleware(h.pool),
	)
	g.GET("/:client_user_id/program-assignment", h.GetProgramAssignment)
	g.PUT("/:client_user_id/program-assignment", h.SetProgramAssignment)
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

	assignment, err := h.programs.GetClientProgramAssignment(c.Request().Context(), trainerUserID, clientUserID)
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

	assignment, err := h.programs.SetClientProgramAssignment(c.Request().Context(), trainerUserID, clientUserID, body.ProgramID)
	if err != nil {
		return program.HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, assignment)
}
