package program

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/exercise"
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
	g := e.Group("/programs",
		auth.JWTMiddleware(h.jwtSecret),
		auth.TrainerMiddleware(h.pool),
	)
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
	g.PATCH("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/publish", h.Publish)
	g.POST("/:id/archive", h.Archive)
	g.POST("/:id/weeks", h.AddWeek)
	g.PUT("/:id/weeks/reorder", h.ReorderWeeks)
	g.DELETE("/:id/weeks/:week_id", h.DeleteWeek)
	g.PUT("/:id/weeks/:week_id/days/reorder", h.ReorderDays)
	g.PUT("/:id/weeks/:week_id/exercises/reorder", h.ReorderWeekExercises)
	g.POST("/:id/weeks/:week_id/days", h.AddDay)
	g.DELETE("/:id/weeks/:week_id/days/:day_id", h.DeleteDay)
	g.POST("/:id/weeks/:week_id/days/:day_id/exercises", h.AddDayExercise)
	g.PUT("/:id/weeks/:week_id/days/:day_id/exercises/:item_id", h.UpdateDayExercise)
	g.DELETE("/:id/weeks/:week_id/days/:day_id/exercises/:item_id", h.DeleteDayExercise)
}

type patchBody struct {
	Name            *string              `json:"name"`
	NameRu          *string              `json:"name_ru"`
	Description     *string              `json:"description"`
	DescriptionRu   *string              `json:"description_ru"`
	Category        *Category            `json:"category"`
	Difficulty      *exercise.Difficulty `json:"difficulty"`
	PreviewImageURL *string              `json:"preview_image_url"`
}

func (b patchBody) toInput() UpdateInput {
	var diff *Difficulty
	if b.Difficulty != nil {
		d := Difficulty(*b.Difficulty)
		diff = &d
	}
	return UpdateInput{
		Name:            b.Name,
		NameRu:          b.NameRu,
		Description:     b.Description,
		DescriptionRu:   b.DescriptionRu,
		Category:        b.Category,
		Difficulty:      diff,
		PreviewImageURL: b.PreviewImageURL,
	}
}

type dayExerciseBody struct {
	ExerciseID  string   `json:"exercise_id"`
	Sets        *int     `json:"sets"`
	Reps        *int     `json:"reps"`
	WeightKg    *float64 `json:"weight_kg"`
	Instruction *string  `json:"instruction"`
}

func (b dayExerciseBody) toInput() (DayExerciseInput, error) {
	exerciseID, err := uuid.Parse(b.ExerciseID)
	if err != nil {
		return DayExerciseInput{}, err
	}
	return DayExerciseInput{
		ExerciseID:  exerciseID,
		Sets:        b.Sets,
		Reps:        b.Reps,
		WeightKg:    b.WeightKg,
		Instruction: b.Instruction,
	}, nil
}

type reorderWeeksBody struct {
	WeekIDs []string `json:"week_ids"`
}

type reorderDaysBody struct {
	DayIDs []string `json:"day_ids"`
}

type reorderExercisesBody struct {
	Days []reorderExercisesDayBody `json:"days"`
}

type reorderExercisesDayBody struct {
	DayID           string   `json:"day_id"`
	ExerciseItemIDs []string `json:"exercise_item_ids"`
}

func parseUUIDList(raw []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (b reorderExercisesBody) toInput() ([]WeekExerciseReorderDay, error) {
	out := make([]WeekExerciseReorderDay, 0, len(b.Days))
	for _, day := range b.Days {
		dayID, err := uuid.Parse(day.DayID)
		if err != nil {
			return nil, err
		}
		itemIDs, err := parseUUIDList(day.ExerciseItemIDs)
		if err != nil {
			return nil, err
		}
		out = append(out, WeekExerciseReorderDay{
			DayID:           dayID,
			ExerciseItemIDs: itemIDs,
		})
	}
	return out, nil
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
		c.QueryParam("status"),
		c.QueryParam("category"),
		c.QueryParam("difficulty"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	result, err := h.svc.List(c.Request().Context(), uid, params)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "list failed")
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) Create(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	d, err := h.svc.Create(c.Request().Context(), uid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "create failed")
	}
	return c.JSON(http.StatusCreated, d)
}

func (h *Handlers) Get(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.Get(c.Request().Context(), uid, id)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) Update(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body patchBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	d, err := h.svc.Update(c.Request().Context(), uid, id, body.toInput())
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) Delete(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	if err := h.svc.Delete(c.Request().Context(), uid, id); err != nil {
		return mapProgramError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handlers) Publish(c echo.Context) error {
	return h.statusAction(c, func(ctx echo.Context, uid, id uuid.UUID) (Detail, error) {
		return h.svc.Publish(ctx.Request().Context(), uid, id)
	})
}

func (h *Handlers) Archive(c echo.Context) error {
	return h.statusAction(c, func(ctx echo.Context, uid, id uuid.UUID) (Detail, error) {
		return h.svc.Archive(ctx.Request().Context(), uid, id)
	})
}

func (h *Handlers) AddWeek(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.AddWeek(c.Request().Context(), uid, programID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) DeleteWeek(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	weekID, err := parseID(c.Param("week_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.DeleteWeek(c.Request().Context(), uid, programID, weekID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) AddDay(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	weekID, err := parseID(c.Param("week_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.AddDay(c.Request().Context(), uid, programID, weekID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) DeleteDay(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	weekID, err := parseID(c.Param("week_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	dayID, err := parseID(c.Param("day_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.DeleteDay(c.Request().Context(), uid, programID, weekID, dayID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) ReorderWeeks(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body reorderWeeksBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	weekIDs, err := parseUUIDList(body.WeekIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.ReorderWeeks(c.Request().Context(), uid, programID, weekIDs)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) ReorderDays(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	weekID, err := parseID(c.Param("week_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body reorderDaysBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	dayIDs, err := parseUUIDList(body.DayIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.ReorderDays(c.Request().Context(), uid, programID, weekID, dayIDs)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) ReorderWeekExercises(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	weekID, err := parseID(c.Param("week_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body reorderExercisesBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	days, err := body.toInput()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.ReorderWeekExercises(c.Request().Context(), uid, programID, weekID, days)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) AddDayExercise(c echo.Context) error {
	return h.dayExerciseAction(c, http.StatusCreated, func(ctx context.Context, uid, programID, weekID, dayID uuid.UUID, in DayExerciseInput) (Detail, error) {
		return h.svc.AddDayExercise(ctx, uid, programID, weekID, dayID, in)
	})
}

func (h *Handlers) UpdateDayExercise(c echo.Context) error {
	return h.dayExerciseAction(c, http.StatusOK, func(ctx context.Context, uid, programID, weekID, dayID uuid.UUID, in DayExerciseInput) (Detail, error) {
		itemID, err := parseID(c.Param("item_id"))
		if err != nil {
			return Detail{}, err
		}
		return h.svc.UpdateDayExercise(ctx, uid, programID, weekID, dayID, itemID, in)
	})
}

func (h *Handlers) DeleteDayExercise(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	weekID, err := parseID(c.Param("week_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	dayID, err := parseID(c.Param("day_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	itemID, err := parseID(c.Param("item_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.DeleteDayExercise(c.Request().Context(), uid, programID, weekID, dayID, itemID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

type dayExerciseFn func(ctx context.Context, uid, programID, weekID, dayID uuid.UUID, in DayExerciseInput) (Detail, error)

func (h *Handlers) dayExerciseAction(c echo.Context, status int, fn dayExerciseFn) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	weekID, err := parseID(c.Param("week_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	dayID, err := parseID(c.Param("day_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body dayExerciseBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	in, err := body.toInput()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := fn(c.Request().Context(), uid, programID, weekID, dayID, in)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(status, d)
}

type statusFn func(c echo.Context, uid, id uuid.UUID) (Detail, error)

func (h *Handlers) statusAction(c echo.Context, fn statusFn) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := fn(c, uid, id)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func parseID(raw string) (uuid.UUID, error) {
	return uuid.Parse(raw)
}

func mapProgramError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, httpx.MsgProgramNotFound)
	case errors.Is(err, ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
	case errors.Is(err, ErrValidation):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidReorder):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidStatusTransition):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, ErrLastWeek), errors.Is(err, ErrLastDay), errors.Is(err, ErrMaxDaysPerWeek):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "program operation failed")
	}
}
