package exercise

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
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
	g := e.Group("/exercises",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerMiddleware(h.pool),
	)
	g.GET("", h.List)
	g.GET("/:id", h.Get)
	g.POST("", h.Create)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
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
	return UpsertInput{
		Name:            b.Name,
		NameRu:          b.NameRu,
		Equipment:       b.Equipment,
		Type:            b.Type,
		MuscleGroup:     b.MuscleGroup,
		Description:     b.Description,
		DescriptionRu:   b.DescriptionRu,
		Difficulty:      b.Difficulty,
		VideoURL:        b.VideoURL,
		PreviewImageURL: b.PreviewImageURL,
	}
}

func (h *Handlers) List(c echo.Context) error {
	items, err := h.svc.List(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "list failed")
	}
	return c.JSON(http.StatusOK, items)
}

func (h *Handlers) Get(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	ex, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "exercise not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "get failed")
	}
	return c.JSON(http.StatusOK, ex)
}

func (h *Handlers) Create(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var body upsertBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid json")
	}
	ex, err := h.svc.Create(c.Request().Context(), uid, body.toInput())
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "create failed")
	}
	return c.JSON(http.StatusCreated, ex)
}

func (h *Handlers) Update(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var body upsertBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid json")
	}
	ex, err := h.svc.Update(c.Request().Context(), id, uid, body.toInput())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "exercise not found")
		}
		if errors.Is(err, ErrValidation) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "update failed")
	}
	return c.JSON(http.StatusOK, ex)
}

func (h *Handlers) Delete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "exercise not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "delete failed")
	}
	return c.NoContent(http.StatusNoContent)
}
