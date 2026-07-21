package workoutcomment

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
	g.POST("/clients/:client_user_id/completions/:completion_id/comments", h.CreateComment)
}

type createCommentRequest struct {
	Text string `json:"text"`
}

func (h *Handlers) CreateComment(c echo.Context) error {
	trainerUserID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	clientUserID, err := uuid.Parse(c.Param("client_user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	completionID, err := uuid.Parse(c.Param("completion_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var req createCommentRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}

	comment, err := h.svc.CreateComment(c.Request().Context(), trainerUserID, clientUserID, completionID, req.Text)
	if err != nil {
		return HTTPErrorFrom(err)
	}
	return c.JSON(http.StatusCreated, comment)
}

func HTTPErrorFrom(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, ErrValidation):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
	case errors.Is(err, ErrCompletionNotFound):
		return echo.NewHTTPError(http.StatusNotFound, ErrCompletionNotFound.Error())
	case errors.Is(err, ErrCommentExists):
		return echo.NewHTTPError(http.StatusConflict, ErrCommentExists.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
}
