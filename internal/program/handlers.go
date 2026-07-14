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
	g.POST("/:id/publish-update", h.PublishUpdate)
	g.POST("/:id/archive", h.Archive)
	g.GET("/:id/assignments", h.ListAssignments)
	g.POST("/:id/assignments/sync", h.SyncAssignments)
	g.GET("/:id/versions", h.ListVersions)
	g.POST("/:id/versions/cleanup", h.CleanupVersions)
	g.DELETE("/:id/versions/:version_id", h.DeleteVersion)
	g.POST("/:id/weeks", h.AddWeek)
	g.PUT("/:id/weeks/reorder", h.ReorderWeeks)
	g.DELETE("/:id/weeks/:week_id", h.DeleteWeek)
	g.PUT("/:id/weeks/:week_id/days/reorder", h.ReorderDays)
	g.PUT("/:id/weeks/:week_id/days/:day_id/blocks/reorder", h.ReorderDayBlocks)
	g.PUT("/:id/weeks/:week_id/blocks/:block_id/exercises/reorder", h.ReorderBlockExercises)
	g.POST("/:id/weeks/:week_id/days", h.AddDay)
	g.DELETE("/:id/weeks/:week_id/days/:day_id", h.DeleteDay)
	g.POST("/:id/weeks/:week_id/days/:day_id/blocks", h.CreateDayBlock)
	g.PUT("/:id/weeks/:week_id/blocks/:block_id/exercises/:item_id", h.UpdateBlockExercise)
	g.DELETE("/:id/weeks/:week_id/blocks/:block_id/exercises/:item_id", h.DeleteBlockExercise)
	g.POST("/:id/weeks/:week_id/days/:day_id/blocks/merge", h.MergeDayBlocks)
	g.PATCH("/:id/weeks/:week_id/blocks/:block_id", h.PatchDayBlock)
	g.POST("/:id/weeks/:week_id/blocks/:block_id/ungroup", h.UngroupDayBlock)
	g.DELETE("/:id/weeks/:week_id/blocks/:block_id", h.DeleteDayBlock)
	g.POST("/:id/weeks/:week_id/blocks/:block_id/exercises", h.AddBlockExercise)
	g.POST("/:id/weeks/:week_id/blocks/:block_id/move", h.MoveDayBlock)
	g.POST("/:id/weeks/:week_id/blocks/:block_id/exercises/:item_id/extract", h.ExtractBlockExercise)
	g.POST("/:id/weeks/:week_id/blocks/:block_id/exercises/:item_id/move", h.MoveExerciseToBlock)
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
	ExerciseID  string  `json:"exercise_id"`
	Sets        *string `json:"sets"`
	Reps        *string `json:"reps"`
	Instruction *string `json:"instruction"`
}

func (b dayExerciseBody) toInput() (DayExerciseInput, error) {
	exerciseID, err := uuid.Parse(b.ExerciseID)
	if err != nil {
		return DayExerciseInput{}, err
	}
	in := DayExerciseInput{
		ExerciseID:  exerciseID,
		Sets:        b.Sets,
		Reps:        b.Reps,
		Instruction: b.Instruction,
	}
	in.NormalizeVolume()
	return in, nil
}

type createDayBlockBody struct {
	BlockType BlockType        `json:"block_type"`
	SortOrder int              `json:"sort_order"`
	Exercise  *dayExerciseBody `json:"exercise"`
}

func (b createDayBlockBody) toInput() (CreateDayBlockInput, error) {
	in := CreateDayBlockInput{
		BlockType: b.BlockType,
		SortOrder: b.SortOrder,
	}
	if b.Exercise != nil {
		ex, err := b.Exercise.toInput()
		if err != nil {
			return CreateDayBlockInput{}, err
		}
		in.Exercise = &ex
	}
	return in, nil
}

type reorderBlocksBody struct {
	BlockIDs []string `json:"block_ids"`
}

type reorderBlockExercisesBody struct {
	ExerciseItemIDs []string `json:"exercise_item_ids"`
}

type mergeBlocksBody struct {
	BlockIDs []string `json:"block_ids"`
}

type patchBlockBody struct {
	BlockType   *BlockType `json:"block_type"`
	Instruction *string    `json:"instruction"`
}

func (b patchBlockBody) toInput() BlockPatchInput {
	return BlockPatchInput(b)
}

type moveBlockBody struct {
	TargetDayID string `json:"target_day_id"`
	SortOrder   *int   `json:"sort_order"`
}

type moveExerciseBody struct {
	TargetBlockID string `json:"target_block_id"`
}

type extractExerciseBody struct {
	SortOrder *int `json:"sort_order"`
}

type reorderWeeksBody struct {
	WeekIDs []string `json:"week_ids"`
}

type reorderDaysBody struct {
	DayIDs []string `json:"day_ids"`
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

func (h *Handlers) PublishUpdate(c echo.Context) error {
	return h.statusAction(c, func(ctx echo.Context, uid, id uuid.UUID) (Detail, error) {
		return h.svc.PublishUpdate(ctx.Request().Context(), uid, id)
	})
}

func (h *Handlers) ListAssignments(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	result, err := h.svc.ListAssignments(c.Request().Context(), uid, programID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) SyncAssignments(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body AssignmentSyncRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	result, err := h.svc.SyncAssignments(c.Request().Context(), uid, programID, body)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) ListVersions(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	result, err := h.svc.ListVersions(c.Request().Context(), uid, programID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) CleanupVersions(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	result, err := h.svc.CleanupVersions(c.Request().Context(), uid, programID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handlers) DeleteVersion(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	versionID, err := parseID(c.Param("version_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	if err := h.svc.DeleteVersion(c.Request().Context(), uid, programID, versionID); err != nil {
		return mapProgramError(err)
	}
	return c.NoContent(http.StatusNoContent)
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

func (h *Handlers) ReorderDayBlocks(c echo.Context) error {
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
	var body reorderBlocksBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	blockIDs, err := parseUUIDList(body.BlockIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.ReorderDayBlocks(c.Request().Context(), uid, programID, weekID, dayID, blockIDs)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) ReorderBlockExercises(c echo.Context) error {
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
	blockID, err := parseID(c.Param("block_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body reorderBlockExercisesBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	itemIDs, err := parseUUIDList(body.ExerciseItemIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.ReorderBlockExercises(c.Request().Context(), uid, programID, weekID, blockID, itemIDs)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) CreateDayBlock(c echo.Context) error {
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
	var body createDayBlockBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	in, err := body.toInput()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.CreateDayBlock(c.Request().Context(), uid, programID, weekID, dayID, in)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusCreated, d)
}

func (h *Handlers) UpdateBlockExercise(c echo.Context) error {
	return h.blockExerciseItemAction(c, http.StatusOK, func(ctx context.Context, uid, programID, weekID, blockID, itemID uuid.UUID, in DayExerciseInput) (Detail, error) {
		return h.svc.UpdateBlockExercise(ctx, uid, programID, weekID, blockID, itemID, in)
	})
}

func (h *Handlers) DeleteBlockExercise(c echo.Context) error {
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
	blockID, err := parseID(c.Param("block_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	itemID, err := parseID(c.Param("item_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.DeleteBlockExercise(c.Request().Context(), uid, programID, weekID, blockID, itemID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

type blockExerciseItemFn func(ctx context.Context, uid, programID, weekID, blockID, itemID uuid.UUID, in DayExerciseInput) (Detail, error)

func (h *Handlers) blockExerciseItemAction(c echo.Context, status int, fn blockExerciseItemFn) error {
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
	blockID, err := parseID(c.Param("block_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	itemID, err := parseID(c.Param("item_id"))
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
	d, err := fn(c.Request().Context(), uid, programID, weekID, blockID, itemID, in)
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

// HTTPErrorFrom maps program domain errors to Echo HTTP errors (shared with trainer client handlers).
func HTTPErrorFrom(err error) *echo.HTTPError {
	return mapProgramError(err)
}

func mapProgramError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, httpx.MsgProgramNotFound)
	case errors.Is(err, ErrReadOnly):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, ErrClientNotLinked):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, ErrClientBlocked), errors.Is(err, ErrProgramNotPublished):
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrClientNotFound):
		return echo.NewHTTPError(http.StatusNotFound, httpx.MsgUserNotFound)
	case errors.Is(err, ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, httpx.MsgForbidden)
	case errors.Is(err, ErrValidation):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidReorder):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidStatusTransition):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoUnpublishedChanges), errors.Is(err, ErrInvalidSyncRequest):
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrVersionHasAssignments), errors.Is(err, ErrSoleProgramVersion):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, ErrLastWeek), errors.Is(err, ErrLastDay), errors.Is(err, ErrMaxDaysPerWeek):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "program operation failed")
	}
}
