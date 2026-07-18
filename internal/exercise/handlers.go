package exercise

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	// Trainers manage their own exercises, admins manage global ones;
	// the service scopes every operation by ownership.
	base := e.Group("/exercises",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerOrAdminMiddleware(h.pool),
	)
	base.GET("", h.List)
	base.GET("/:id", h.Get)
	base.POST("", h.Create)
	base.DELETE("", h.DeleteMany)
	base.PUT("/:id", h.Update)
}

type deleteManyResponse struct {
	DeletedCount int64 `json:"deleted_count"`
}

type deleteManyBody struct {
	IDs []string `json:"ids"`
}

type upsertBody struct {
	Name            string       `json:"name"`
	NameRu          string       `json:"name_ru"`
	Equipment       *Equipment   `json:"equipment"`
	Type            ExerciseType `json:"type"`
	MuscleGroup     MuscleGroup  `json:"muscle_group"`
	Description     string       `json:"description"`
	DescriptionRu   string       `json:"description_ru"`
	Difficulty      Difficulty   `json:"difficulty"`
	VideoURL        string       `json:"video_url"`
	PreviewImageURL string       `json:"preview_image_url"`
}

func (b upsertBody) toInput() UpsertInput {
	return UpsertInput(b)
}

func (h *Handlers) List(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	params, err := ParseListParams(
		c.QueryParam("page"),
		c.QueryParam("limit"),
		c.QueryParam("sort_by"),
		c.QueryParam("sort_order"),
		c.QueryParam("q"),
		c.QueryParam("type"),
		c.QueryParam("muscle_group"),
		c.QueryParam("difficulty"),
		c.QueryParam("equipment"),
		c.QueryParam("scope"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := h.svc.List(c.Request().Context(), uid, params)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "list failed")
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) Get(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	ex, err := h.svc.Get(c.Request().Context(), uid, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgExerciseNotFound)
		}
		if errors.Is(err, ErrForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "get failed")
	}
	return c.JSON(http.StatusOK, ex)
}

func (h *Handlers) Create(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	var body upsertBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	ex, err := h.svc.Create(c.Request().Context(), uid, body.toInput())
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if errors.Is(err, ErrForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
		}
		var qe *subscription.QuotaError
		if errors.As(err, &qe) {
			return subscription.QuotaHTTPError(qe)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "create failed")
	}
	return c.JSON(http.StatusCreated, ex)
}

func (h *Handlers) Update(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body upsertBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	ex, err := h.svc.Update(c.Request().Context(), id, uid, body.toInput())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgExerciseNotFound)
		}
		if errors.Is(err, ErrValidation) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if errors.Is(err, ErrForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
		}
		var qe *subscription.QuotaError
		if errors.As(err, &qe) {
			return subscription.QuotaHTTPError(qe)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "update failed")
	}
	return c.JSON(http.StatusOK, ex)
}

func (h *Handlers) DeleteMany(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	var body deleteManyBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if len(body.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "ids is required")
	}

	seen := make(map[uuid.UUID]struct{}, len(body.IDs))
	ids := make([]uuid.UUID, 0, len(body.IDs))
	for _, raw := range body.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	count, err := h.svc.DeleteMany(c.Request().Context(), uid, ids)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "delete failed")
	}
	return c.JSON(http.StatusOK, deleteManyResponse{DeletedCount: count})
}
