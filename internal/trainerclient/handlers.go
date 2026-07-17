package trainerclient

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/telegram"
)

type Handlers struct {
	svc        *Service
	pool       *pgxpool.Pool
	jwtSecret  string
	httpClient *http.Client
}

func NewHandlers(svc *Service, pool *pgxpool.Pool, jwtSecret string) *Handlers {
	return &Handlers{
		svc:        svc,
		pool:       pool,
		jwtSecret:  jwtSecret,
		httpClient: http.DefaultClient,
	}
}

func (h *Handlers) Mount(e *echo.Echo) {
	trainer := e.Group("/trainer",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerMiddleware(h.pool),
	)
	trainer.POST("/invites", h.CreateInvite)

	clients := e.Group("/trainer/clients", auth.JWTMiddleware(h.jwtSecret))
	// Admins have read-only access to the full client list; management stays trainer-only.
	clients.GET("", h.ListClients, auth.TrainerOrAdminMiddleware(h.pool))
	clients.PUT("/program-assignment", h.SetProgramAssignment, auth.TrainerMiddleware(h.pool))
	clients.GET("/:client_user_id/program-assignment", h.GetProgramAssignment, auth.TrainerMiddleware(h.pool))

	e.GET("/trainer/clients/:client_user_id/avatar", h.GetClientAvatar)
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

	var body program.BulkSetClientProgramAssignmentRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}

	result, err := h.svc.BulkSetClientProgramAssignment(c.Request().Context(), trainerUserID, body)
	if err != nil {
		return program.HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) GetClientAvatar(c echo.Context) error {
	clientUserID, err := uuid.Parse(c.Param("client_user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}

	exp, err := strconv.ParseInt(c.QueryParam("exp"), 10, 64)
	if err != nil || !VerifyAvatarURL(h.jwtSecret, clientUserID, exp, c.QueryParam("sig")) {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}

	filePath, err := h.svc.ClientAvatarFilePath(c.Request().Context(), clientUserID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	if filePath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "avatar not found")
	}

	botToken := h.svc.BotToken()
	if botToken == "" {
		return echo.NewHTTPError(http.StatusNotFound, "avatar not found")
	}

	if err := telegram.StreamBotFile(c.Response(), h.httpClient, botToken, filePath); err != nil {
		if errors.Is(err, telegram.ErrFileNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "avatar not found")
		}
		return echo.NewHTTPError(http.StatusBadGateway, httpx.MsgInternal)
	}
	return nil
}
