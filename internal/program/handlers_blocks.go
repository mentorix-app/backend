package program

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
)

func (h *Handlers) AddBlockExercise(c echo.Context) error {
	return h.blockExerciseAction(c, http.StatusCreated, func(ctx context.Context, uid, programID, weekID, blockID uuid.UUID, in DayExerciseInput) (Detail, error) {
		return h.svc.AddBlockExercise(ctx, uid, programID, weekID, blockID, in)
	})
}

func (h *Handlers) PatchDayBlock(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, weekID, blockID, err := parseWeekBlockIDs(c)
	if err != nil {
		return err
	}
	var body patchBlockBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	d, err := h.svc.PatchDayBlock(c.Request().Context(), uid, programID, weekID, blockID, body.toInput())
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) MergeDayBlocks(c echo.Context) error {
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
	var body mergeBlocksBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	blockIDs, err := parseUUIDList(body.BlockIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.MergeDayBlocks(c.Request().Context(), uid, programID, weekID, dayID, blockIDs)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) UngroupDayBlock(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, weekID, blockID, err := parseWeekBlockIDs(c)
	if err != nil {
		return err
	}
	d, err := h.svc.UngroupDayBlock(c.Request().Context(), uid, programID, weekID, blockID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) DeleteDayBlock(c echo.Context) error {
	uid, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	programID, weekID, blockID, err := parseWeekBlockIDs(c)
	if err != nil {
		return err
	}
	d, err := h.svc.DeleteDayBlock(c.Request().Context(), uid, programID, weekID, blockID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) MoveDayBlock(c echo.Context) error {
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
	var body moveBlockBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	targetDayID, err := uuid.Parse(body.TargetDayID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	insertSort := 0
	if body.SortOrder != nil {
		insertSort = *body.SortOrder
	}
	d, err := h.svc.MoveDayBlock(c.Request().Context(), uid, programID, weekID, blockID, targetDayID, insertSort)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) ExtractBlockExercise(c echo.Context) error {
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
	var body extractExerciseBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	insertSort := 0
	if body.SortOrder != nil {
		insertSort = *body.SortOrder
	}
	d, err := h.svc.ExtractBlockExercise(c.Request().Context(), uid, programID, weekID, blockID, itemID, insertSort)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

func (h *Handlers) MoveExerciseToBlock(c echo.Context) error {
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
	var body moveExerciseBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	targetBlockID, err := uuid.Parse(body.TargetBlockID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := h.svc.MoveExerciseToBlock(c.Request().Context(), uid, programID, weekID, blockID, itemID, targetBlockID)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(http.StatusOK, d)
}

type blockExerciseFn func(ctx context.Context, uid, programID, weekID, blockID uuid.UUID, in DayExerciseInput) (Detail, error)

func (h *Handlers) blockExerciseAction(c echo.Context, status int, fn blockExerciseFn) error {
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
	var body dayExerciseBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	in, err := body.toInput()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	d, err := fn(c.Request().Context(), uid, programID, weekID, blockID, in)
	if err != nil {
		return mapProgramError(err)
	}
	return c.JSON(status, d)
}

func parseWeekBlockIDs(c echo.Context) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	programID, err := parseID(c.Param("id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	weekID, err := parseID(c.Param("week_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	blockID, err := parseID(c.Param("block_id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	return programID, weekID, blockID, nil
}
