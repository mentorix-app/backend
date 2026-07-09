package program

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

func blockHandlerFixture() (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, *Handlers) {
	userID := uuid.New()
	programID := uuid.New()
	detail := sampleDetail(userID, programID)
	weekID := detail.Weeks[0].ID
	dayID := detail.Weeks[0].Days[0].ID
	blockID := uuid.New()
	store := &fakeProgramStore{program: detail.Program, detail: detail}
	return userID, programID, weekID, dayID, blockID, programHandler(store)
}

func TestHandlers_PatchDayBlock(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	body := `{"block_type":"emom","instruction":"60s work"}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPatch, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String(), body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	if err := h.PatchDayBlock(c); err != nil {
		t.Fatalf("PatchDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_MergeDayBlocks(t *testing.T) {
	userID, programID, weekID, dayID, blockID, h := blockHandlerFixture()
	body := `{"block_ids":["` + blockID.String() + `","` + uuid.New().String() + `"]}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/"+dayID.String()+"/blocks/merge", body, userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
		"day_id":  dayID.String(),
	})
	if err := h.MergeDayBlocks(c); err != nil {
		t.Fatalf("MergeDayBlocks: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_UngroupDayBlock(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/ungroup", "", userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	if err := h.UngroupDayBlock(c); err != nil {
		t.Fatalf("UngroupDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_DeleteDayBlock(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	e := echo.New()
	c, rec := programContext(e, http.MethodDelete, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String(), "", userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	if err := h.DeleteDayBlock(c); err != nil {
		t.Fatalf("DeleteDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_MoveDayBlock(t *testing.T) {
	userID, programID, weekID, dayID, blockID, h := blockHandlerFixture()
	body := `{"target_day_id":"` + dayID.String() + `","sort_order":1}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/move", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	if err := h.MoveDayBlock(c); err != nil {
		t.Fatalf("MoveDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_AddBlockExercise_nullSetsReps(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	exerciseID := uuid.New()
	body := `{"exercise_id":"` + exerciseID.String() + `","sets":null,"reps":null}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	if err := h.AddBlockExercise(c); err != nil {
		t.Fatalf("AddBlockExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
}

func TestHandlers_AddBlockExercise_zeroSets(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	exerciseID := uuid.New()
	body := `{"exercise_id":"` + exerciseID.String() + `","sets":0}`
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	assertHTTPError(t, h.AddBlockExercise(c), http.StatusBadRequest)
}

func TestHandlers_AddBlockExercise_negativeSets(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	exerciseID := uuid.New()
	body := `{"exercise_id":"` + exerciseID.String() + `","sets":-1}`
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	assertHTTPError(t, h.AddBlockExercise(c), http.StatusBadRequest)
}

func TestHandlers_AddBlockExercise(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	exerciseID := uuid.New()
	body := `{"exercise_id":"` + exerciseID.String() + `","sets":3,"reps":10}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	if err := h.AddBlockExercise(c); err != nil {
		t.Fatalf("AddBlockExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
}

func TestHandlers_ExtractBlockExercise(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	itemID := uuid.New()
	body := `{"sort_order":1}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises/"+itemID.String()+"/extract", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
		"item_id":  itemID.String(),
	})
	if err := h.ExtractBlockExercise(c); err != nil {
		t.Fatalf("ExtractBlockExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_MoveExerciseToBlock(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	itemID := uuid.New()
	targetBlockID := uuid.New()
	body := `{"target_block_id":"` + targetBlockID.String() + `"}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises/"+itemID.String()+"/move", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
		"item_id":  itemID.String(),
	})
	if err := h.MoveExerciseToBlock(c); err != nil {
		t.Fatalf("MoveExerciseToBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_ReorderDayBlocks(t *testing.T) {
	userID, programID, weekID, dayID, blockID, h := blockHandlerFixture()
	body := `{"block_ids":["` + blockID.String() + `"]}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/"+dayID.String()+"/blocks/reorder", body, userID, map[string]string{
		"id":      programID.String(),
		"week_id": weekID.String(),
		"day_id":  dayID.String(),
	})
	if err := h.ReorderDayBlocks(c); err != nil {
		t.Fatalf("ReorderDayBlocks: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestPatchBlockBody_toInput(t *testing.T) {
	blockType := BlockTypeEMOM
	instr := "work"
	got := patchBlockBody{BlockType: &blockType, Instruction: &instr}.toInput()
	if got.BlockType == nil || *got.BlockType != BlockTypeEMOM || got.Instruction == nil || *got.Instruction != instr {
		t.Fatalf("toInput() = %+v", got)
	}
}

func TestHandlers_blockEndpoints_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	programID := uuid.New()
	weekID := uuid.New()
	dayID := uuid.New()
	blockID := uuid.New()
	itemID := uuid.New()

	cases := []struct {
		name string
		run  func() error
	}{
		{"PatchDayBlock", func() error {
			req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"block_type":"emom"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String())
			return h.PatchDayBlock(c)
		}},
		{"MergeDayBlocks", func() error {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"block_ids":[]}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "day_id")
			c.SetParamValues(programID.String(), weekID.String(), dayID.String())
			return h.MergeDayBlocks(c)
		}},
		{"UngroupDayBlock", func() error {
			c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", nil), httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String())
			return h.UngroupDayBlock(c)
		}},
		{"DeleteDayBlock", func() error {
			c := e.NewContext(httptest.NewRequest(http.MethodDelete, "/", nil), httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String())
			return h.DeleteDayBlock(c)
		}},
		{"MoveDayBlock", func() error {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"target_day_id":"`+dayID.String()+`"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String())
			return h.MoveDayBlock(c)
		}},
		{"AddBlockExercise", func() error {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"exercise_id":"`+uuid.New().String()+`"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String())
			return h.AddBlockExercise(c)
		}},
		{"ExtractBlockExercise", func() error {
			c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", nil), httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id", "item_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String(), itemID.String())
			return h.ExtractBlockExercise(c)
		}},
		{"MoveExerciseToBlock", func() error {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"target_block_id":"`+blockID.String()+`"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id", "item_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String(), itemID.String())
			return h.MoveExerciseToBlock(c)
		}},
		{"ReorderDayBlocks", func() error {
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"block_ids":[]}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "day_id")
			c.SetParamValues(programID.String(), weekID.String(), dayID.String())
			return h.ReorderDayBlocks(c)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertHTTPError(t, tc.run(), http.StatusUnauthorized)
		})
	}
}

func TestHandlers_blockEndpoints_invalidID(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	weekID := uuid.New()
	dayID := uuid.New()
	blockID := uuid.New()

	cases := []struct {
		name string
		run  func() error
	}{
		{"PatchDayBlock", func() error {
			c, _ := programContext(e, http.MethodPatch, "/", `{"block_type":"emom"}`, userID, map[string]string{
				"id": "bad", "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.PatchDayBlock(c)
		}},
		{"MergeDayBlocks invalid day", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"block_ids":["`+blockID.String()+`"]}`, userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "day_id": "bad",
			})
			return h.MergeDayBlocks(c)
		}},
		{"MoveDayBlock invalid target", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"target_day_id":"bad"}`, userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.MoveDayBlock(c)
		}},
		{"AddBlockExercise invalid exercise", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"exercise_id":"bad","sets":3,"reps":10}`, userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.AddBlockExercise(c)
		}},
		{"ReorderDayBlocks invalid block id", func() error {
			c, _ := programContext(e, http.MethodPut, "/", `{"block_ids":["bad"]}`, userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "day_id": dayID.String(),
			})
			return h.ReorderDayBlocks(c)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertHTTPError(t, tc.run(), http.StatusBadRequest)
		})
	}
}

func TestHandlers_ReorderDayBlocks_mapsInvalidReorder(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	dayID := uuid.New()
	store := &reorderDayBlocksErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		},
		reorderErr: ErrInvalidReorder,
	}
	h := programHandler(store)
	body := `{"block_ids":["` + uuid.New().String() + `"]}`
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/days/"+dayID.String()+"/blocks/reorder", body, userID, map[string]string{
		"id": programID.String(), "week_id": weekID.String(), "day_id": dayID.String(),
	})
	assertHTTPError(t, h.ReorderDayBlocks(c), http.StatusBadRequest)
}

func TestHandlers_UpdateBlockExercise_invalidJSON(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	blockID := uuid.New()
	itemID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPut, "/", "{", userID, map[string]string{
		"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(), "item_id": itemID.String(),
	})
	assertHTTPError(t, h.UpdateBlockExercise(c), http.StatusBadRequest)
}

func TestHandlers_UpdateBlockExercise_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"sets":3}`)), httptest.NewRecorder())
	c.SetParamNames("id", "week_id", "block_id", "item_id")
	c.SetParamValues(uuid.New().String(), uuid.New().String(), uuid.New().String(), uuid.New().String())
	assertHTTPError(t, h.UpdateBlockExercise(c), http.StatusUnauthorized)
}

func TestHandlers_PatchDayBlock_invalidJSON(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPatch, "/", "{", userID, map[string]string{
		"id": uuid.New().String(), "week_id": uuid.New().String(), "block_id": uuid.New().String(),
	})
	assertHTTPError(t, h.PatchDayBlock(c), http.StatusBadRequest)
}

func TestHandlers_MergeDayBlocks_invalidJSON(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/", "{", userID, map[string]string{
		"id": uuid.New().String(), "week_id": uuid.New().String(), "day_id": uuid.New().String(),
	})
	assertHTTPError(t, h.MergeDayBlocks(c), http.StatusBadRequest)
}

func TestHandlers_AddBlockExercise_invalidJSON(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/", "{", userID, map[string]string{
		"id": uuid.New().String(), "week_id": uuid.New().String(), "block_id": uuid.New().String(),
	})
	assertHTTPError(t, h.AddBlockExercise(c), http.StatusBadRequest)
}

func TestHandlers_MoveDayBlock_invalidJSON(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/", "{", userID, map[string]string{
		"id": uuid.New().String(), "week_id": uuid.New().String(), "block_id": uuid.New().String(),
	})
	assertHTTPError(t, h.MoveDayBlock(c), http.StatusBadRequest)
}

func TestHandlers_DeleteBlockExercise_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodDelete, "/", nil), httptest.NewRecorder())
	c.SetParamNames("id", "week_id", "block_id", "item_id")
	c.SetParamValues(uuid.New().String(), uuid.New().String(), uuid.New().String(), uuid.New().String())
	assertHTTPError(t, h.DeleteBlockExercise(c), http.StatusUnauthorized)
}

func TestHandlers_DeleteBlockExercise_invalidID(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodDelete, "/", "", userID, map[string]string{
		"id": "bad", "week_id": uuid.New().String(), "block_id": uuid.New().String(), "item_id": uuid.New().String(),
	})
	assertHTTPError(t, h.DeleteBlockExercise(c), http.StatusBadRequest)
}

func TestHandlers_ReorderDayBlocks_unauthorized(t *testing.T) {
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"block_ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("id", "week_id", "day_id")
	c.SetParamValues(uuid.New().String(), uuid.New().String(), uuid.New().String())
	assertHTTPError(t, h.ReorderDayBlocks(c), http.StatusUnauthorized)
}

func TestHandlers_blockEndpoints_notFound(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	dayID := uuid.New()
	blockID := uuid.New()
	itemID := uuid.New()
	store := &fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
		err:     pgx.ErrNoRows,
	}
	h := programHandler(store)
	e := echo.New()

	cases := []struct {
		name string
		run  func() error
	}{
		{"PatchDayBlock", func() error {
			c, _ := programContext(e, http.MethodPatch, "/", `{"block_type":"emom"}`, userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.PatchDayBlock(c)
		}},
		{"MergeDayBlocks", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"block_ids":["`+blockID.String()+`"]}`, userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "day_id": dayID.String(),
			})
			return h.MergeDayBlocks(c)
		}},
		{"UngroupDayBlock", func() error {
			c, _ := programContext(e, http.MethodPost, "/", "", userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.UngroupDayBlock(c)
		}},
		{"DeleteDayBlock", func() error {
			c, _ := programContext(e, http.MethodDelete, "/", "", userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.DeleteDayBlock(c)
		}},
		{"MoveDayBlock", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"target_day_id":"`+dayID.String()+`"}`, userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.MoveDayBlock(c)
		}},
		{"AddBlockExercise", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"exercise_id":"`+uuid.New().String()+`","sets":3,"reps":10}`, userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.AddBlockExercise(c)
		}},
		{"ExtractBlockExercise", func() error {
			c, _ := programContext(e, http.MethodPost, "/", "", userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(), "item_id": itemID.String(),
			})
			return h.ExtractBlockExercise(c)
		}},
		{"MoveExerciseToBlock", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"target_block_id":"`+blockID.String()+`"}`, userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(), "item_id": itemID.String(),
			})
			return h.MoveExerciseToBlock(c)
		}},
		{"ReorderDayBlocks", func() error {
			c, _ := programContext(e, http.MethodPut, "/", `{"block_ids":["`+blockID.String()+`"]}`, userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "day_id": dayID.String(),
			})
			return h.ReorderDayBlocks(c)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertHTTPError(t, tc.run(), http.StatusNotFound)
		})
	}
}
