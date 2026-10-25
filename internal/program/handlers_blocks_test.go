package program

import (
	"context"
	"fmt"
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
	body := `{"exercise_id":"` + exerciseID.String() + `","sets":"3/"}`
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
	body := `{"exercise_id":"` + exerciseID.String() + `","sets":"abc"}`
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
	body := `{"exercise_id":"` + exerciseID.String() + `","sets":"3","reps":"10"}`
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

type duplicateCaptureStore struct {
	fakeProgramStore
	programID, weekID, blockID, itemID uuid.UUID
}

func (s *duplicateCaptureStore) DuplicateBlockExercise(_ context.Context, _, programID, weekID, blockID, itemID uuid.UUID) (Detail, error) {
	s.programID, s.weekID, s.blockID, s.itemID = programID, weekID, blockID, itemID
	return s.detail, nil
}

func TestHandlers_DuplicateBlockExercise(t *testing.T) {
	userID := uuid.New()
	programID, weekID, blockID, itemID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	store := &duplicateCaptureStore{fakeProgramStore: fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}}
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/exercises/"+itemID.String()+"/duplicate", "", userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
		"item_id":  itemID.String(),
	})
	if err := programHandler(store).DuplicateBlockExercise(c); err != nil {
		t.Fatalf("DuplicateBlockExercise: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
	got := [4]uuid.UUID{store.programID, store.weekID, store.blockID, store.itemID}
	if want := [4]uuid.UUID{programID, weekID, blockID, itemID}; got != want {
		t.Fatalf("store got program/week/block/item %v, want %v", got, want)
	}
}

func TestHandlers_CopyDayBlock(t *testing.T) {
	userID, programID, weekID, dayID, blockID, h := blockHandlerFixture()
	body := `{"target_day_id":"` + dayID.String() + `"}`
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String()+"/copy", body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	if err := h.CopyDayBlock(c); err != nil {
		t.Fatalf("CopyDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

type copyCaptureStore struct {
	fakeProgramStore
	blockID, targetDayID uuid.UUID
}

func (s *copyCaptureStore) CopyDayBlock(_ context.Context, _, _, _, blockID, targetDayID uuid.UUID) (Detail, error) {
	s.blockID, s.targetDayID = blockID, targetDayID
	return s.detail, nil
}

func TestHandlers_CopyDayBlock_passesBlockAndTargetDay(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	blockID := uuid.New()
	targetDayID := uuid.New()
	store := &copyCaptureStore{fakeProgramStore: fakeProgramStore{
		program: Program{ID: programID, CreatedBy: userID, Status: StatusDraft},
	}}
	e := echo.New()
	c, rec := programContext(e, http.MethodPost, "/", `{"target_day_id":"`+targetDayID.String()+`"}`, userID, map[string]string{
		"id": programID.String(), "week_id": uuid.New().String(), "block_id": blockID.String(),
	})
	if err := programHandler(store).CopyDayBlock(c); err != nil {
		t.Fatalf("CopyDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
	if store.blockID != blockID || store.targetDayID != targetDayID {
		t.Fatalf("store got block %s target %s, want %s and %s", store.blockID, store.targetDayID, blockID, targetDayID)
	}
}

func TestHandlers_CopyDayBlock_invalidJSON(t *testing.T) {
	userID := uuid.New()
	h := programHandler(&fakeProgramStore{})
	e := echo.New()
	c, _ := programContext(e, http.MethodPost, "/", "{", userID, map[string]string{
		"id": uuid.New().String(), "week_id": uuid.New().String(), "block_id": uuid.New().String(),
	})
	assertHTTPError(t, h.CopyDayBlock(c), http.StatusBadRequest)
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

type patchCaptureStore struct {
	fakeProgramStore
	got BlockPatchInput
	err error
}

func (s *patchCaptureStore) PatchDayBlock(_ context.Context, _, _, _, _ uuid.UUID, in BlockPatchInput) (Detail, error) {
	s.got = in
	return s.detail, s.err
}

func patchBlockRequest(t *testing.T, store *patchCaptureStore, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	userID := uuid.New()
	programID := uuid.New()
	weekID := uuid.New()
	blockID := uuid.New()
	store.program = Program{ID: programID, CreatedBy: userID, Status: StatusDraft}
	e := echo.New()
	c, rec := programContext(e, http.MethodPatch, "/programs/"+programID.String()+"/weeks/"+weekID.String()+"/blocks/"+blockID.String(), body, userID, map[string]string{
		"id":       programID.String(),
		"week_id":  weekID.String(),
		"block_id": blockID.String(),
	})
	return rec, programHandler(store).PatchDayBlock(c)
}

func TestHandlers_PatchDayBlock_passesTriStateFields(t *testing.T) {
	store := &patchCaptureStore{}
	body := `{"rounds":8,"duration_sec":null,"rest_sec":0}`
	rec, err := patchBlockRequest(t, store, body)
	if err != nil {
		t.Fatalf("PatchDayBlock: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)

	got := store.got
	if !got.Rounds.Present || got.Rounds.Value == nil || *got.Rounds.Value != 8 {
		t.Fatalf("rounds = %+v, want set to 8", got.Rounds)
	}
	if !got.DurationSec.Present || got.DurationSec.Value != nil {
		t.Fatalf("duration_sec = %+v, want present and null", got.DurationSec)
	}
	if !got.RestSec.Present || got.RestSec.Value == nil || *got.RestSec.Value != 0 {
		t.Fatalf("rest_sec = %+v, want set to 0", got.RestSec)
	}
	if got.TimeCapSec.Present || got.WorkSec.Present {
		t.Fatalf("absent keys must stay absent: time_cap_sec=%+v work_sec=%+v", got.TimeCapSec, got.WorkSec)
	}
}

func TestHandlers_PatchDayBlock_rejectsBadBodies(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		storeErr  error
		wantCode  int
		wantStore bool
	}{
		{"string value", `{"rounds":"8"}`, nil, http.StatusBadRequest, false},
		{"fractional value", `{"rounds":8.5}`, nil, http.StatusBadRequest, false},
		{"bool value", `{"work_sec":true}`, nil, http.StatusBadRequest, false},
		{"rounds above max", `{"rounds":101}`, nil, http.StatusBadRequest, false},
		{"rounds zero", `{"rounds":0}`, nil, http.StatusBadRequest, false},
		{"duration above max", `{"duration_sec":86401}`, nil, http.StatusBadRequest, false},
		{"rest negative", `{"rest_sec":-1}`, nil, http.StatusBadRequest, false},
		{"removed block type", `{"block_type":"complex"}`, nil, http.StatusBadRequest, false},
		{"unknown block type", `{"block_type":"nope"}`, nil, http.StatusBadRequest, false},
		{"field not allowed for the type", `{"duration_sec":600}`, fmt.Errorf("%w: duration_sec is not allowed for block_type emom", ErrValidation), http.StatusBadRequest, true},
		{"block not found", `{"rounds":3}`, pgx.ErrNoRows, http.StatusNotFound, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &patchCaptureStore{err: tt.storeErr}
			_, err := patchBlockRequest(t, store, tt.body)
			assertHTTPError(t, err, tt.wantCode)
			if called := store.got != (BlockPatchInput{}); called != tt.wantStore {
				t.Fatalf("store called = %v, want %v", called, tt.wantStore)
			}
		})
	}
}

func TestPatchBlockBody_toInput(t *testing.T) {
	blockType := BlockTypeEMOM
	instr := "work"
	rounds := 6
	got := DayBlockPatchRequest{
		BlockType:   &blockType,
		Instruction: &instr,
		Rounds:      OptionalInt{Present: true, Value: &rounds},
		RestSec:     OptionalInt{Present: true},
	}.toInput()
	if got.BlockType == nil || *got.BlockType != BlockTypeEMOM || got.Instruction == nil || *got.Instruction != instr {
		t.Fatalf("toInput() = %+v", got)
	}
	if !got.Rounds.Present || got.Rounds.Value == nil || *got.Rounds.Value != 6 {
		t.Fatalf("rounds = %+v", got.Rounds)
	}
	if !got.RestSec.Present || got.RestSec.Value != nil || got.DurationSec.Present {
		t.Fatalf("rest_sec/duration_sec = %+v / %+v", got.RestSec, got.DurationSec)
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
		{"DuplicateBlockExercise", func() error {
			c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", nil), httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id", "item_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String(), itemID.String())
			return h.DuplicateBlockExercise(c)
		}},
		{"CopyDayBlock", func() error {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"target_day_id":"`+dayID.String()+`"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "week_id", "block_id")
			c.SetParamValues(programID.String(), weekID.String(), blockID.String())
			return h.CopyDayBlock(c)
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
		{"CopyDayBlock invalid target", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"target_day_id":"bad"}`, userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.CopyDayBlock(c)
		}},
		{"CopyDayBlock missing target", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{}`, userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.CopyDayBlock(c)
		}},
		{"CopyDayBlock invalid block", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"target_day_id":"`+dayID.String()+`"}`, userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "block_id": "bad",
			})
			return h.CopyDayBlock(c)
		}},
		{"CopyDayBlock invalid program", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"target_day_id":"`+dayID.String()+`"}`, userID, map[string]string{
				"id": "bad", "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.CopyDayBlock(c)
		}},
		{"CopyDayBlock invalid week", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"target_day_id":"`+dayID.String()+`"}`, userID, map[string]string{
				"id": uuid.New().String(), "week_id": "bad", "block_id": blockID.String(),
			})
			return h.CopyDayBlock(c)
		}},
		{"DuplicateBlockExercise invalid program", func() error {
			c, _ := programContext(e, http.MethodPost, "/", "", userID, map[string]string{
				"id": "bad", "week_id": weekID.String(), "block_id": blockID.String(), "item_id": uuid.New().String(),
			})
			return h.DuplicateBlockExercise(c)
		}},
		{"DuplicateBlockExercise invalid week", func() error {
			c, _ := programContext(e, http.MethodPost, "/", "", userID, map[string]string{
				"id": uuid.New().String(), "week_id": "bad", "block_id": blockID.String(), "item_id": uuid.New().String(),
			})
			return h.DuplicateBlockExercise(c)
		}},
		{"DuplicateBlockExercise invalid block", func() error {
			c, _ := programContext(e, http.MethodPost, "/", "", userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "block_id": "bad", "item_id": uuid.New().String(),
			})
			return h.DuplicateBlockExercise(c)
		}},
		{"DuplicateBlockExercise invalid item", func() error {
			c, _ := programContext(e, http.MethodPost, "/", "", userID, map[string]string{
				"id": uuid.New().String(), "week_id": weekID.String(), "block_id": blockID.String(), "item_id": "bad",
			})
			return h.DuplicateBlockExercise(c)
		}},
		{"AddBlockExercise invalid exercise", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"exercise_id":"bad","sets":"3","reps":"10"}`, userID, map[string]string{
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
	c := e.NewContext(httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"sets":"3"}`)), httptest.NewRecorder())
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
			c, _ := programContext(e, http.MethodPost, "/", `{"exercise_id":"`+uuid.New().String()+`","sets":"3","reps":"10"}`, userID, map[string]string{
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
		{"DuplicateBlockExercise", func() error {
			c, _ := programContext(e, http.MethodPost, "/", "", userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(), "item_id": itemID.String(),
			})
			return h.DuplicateBlockExercise(c)
		}},
		{"CopyDayBlock", func() error {
			c, _ := programContext(e, http.MethodPost, "/", `{"target_day_id":"`+dayID.String()+`"}`, userID, map[string]string{
				"id": programID.String(), "week_id": weekID.String(), "block_id": blockID.String(),
			})
			return h.CopyDayBlock(c)
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

type setBlockClientsErrStore struct {
	fakeProgramStore
	setErr error
}

func (s *setBlockClientsErrStore) SetBlockClients(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, []uuid.UUID) (Detail, error) {
	return Detail{}, s.setErr
}

func setBlockClientsContext(t *testing.T, userID, programID, weekID, blockID uuid.UUID, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	return programContext(e, http.MethodPut,
		"/programs/"+programID.String()+"/weeks/"+weekID.String()+
			"/blocks/"+blockID.String()+"/clients",
		body, userID, map[string]string{
			"id":       programID.String(),
			"week_id":  weekID.String(),
			"block_id": blockID.String(),
		})
}

func TestHandlers_SetBlockClients(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	body := `{"client_user_ids":["` + uuid.New().String() + `"]}`
	c, rec := setBlockClientsContext(t, userID, programID, weekID, blockID, body)
	if err := h.SetBlockClients(c); err != nil {
		t.Fatalf("SetBlockClients: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestHandlers_SetBlockClients_mapsLastSharedBlock(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &setBlockClientsErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		setErr: fmt.Errorf("%w: week 1 day 3 in working copy", ErrLastSharedBlock),
	}
	h := programHandler(store)
	body := `{"client_user_ids":["` + uuid.New().String() + `"]}`
	c, _ := setBlockClientsContext(t, userID, programID, uuid.New(), uuid.New(), body)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}

func TestHandlers_SetBlockClients_mapsClientNotAssigned(t *testing.T) {
	userID := uuid.New()
	programID := uuid.New()
	store := &setBlockClientsErrStore{
		fakeProgramStore: fakeProgramStore{
			program: Program{ID: programID, CreatedBy: userID, Status: StatusPublished},
		},
		setErr: ErrClientNotAssignedToProgram,
	}
	h := programHandler(store)
	body := `{"client_user_ids":["` + uuid.New().String() + `"]}`
	c, _ := setBlockClientsContext(t, userID, programID, uuid.New(), uuid.New(), body)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}

func TestHandlers_SetBlockClients_rejectsDuplicateIDs(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	dup := uuid.New().String()
	body := `{"client_user_ids":["` + dup + `","` + dup + `"]}`
	c, _ := setBlockClientsContext(t, userID, programID, weekID, blockID, body)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}

func TestHandlers_SetBlockClients_rejectsNonUUID(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	body := `{"client_user_ids":["not-a-uuid"]}`
	c, _ := setBlockClientsContext(t, userID, programID, weekID, blockID, body)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}

func TestHandlers_SetBlockClients_rejectsMissingClientUserIDs(t *testing.T) {
	userID, programID, weekID, _, blockID, h := blockHandlerFixture()
	c, _ := setBlockClientsContext(t, userID, programID, weekID, blockID, `{}`)
	assertHTTPError(t, h.SetBlockClients(c), http.StatusBadRequest)
}
