package trainerclient

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/auth"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/workoutcompletion"
)

// ClientGetProgram answers with the caller's program from one trainer. The user
// comes only from the access token and the trainer only from the path.
func (h *Handlers) ClientGetProgram(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	trainerID, err := uuid.Parse(c.Param("trainer_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	resp, err := h.clientApp.GetClientProgram(c.Request().Context(), userID, trainerID)
	if err != nil {
		return clientAppError(err)
	}
	if resp.CompletedDays == nil {
		resp.CompletedDays = []ClientCompletedDay{}
	}
	return c.JSON(http.StatusOK, resp)
}

// ClientCompleteDay marks one day of the caller's current program done.
func (h *Handlers) ClientCompleteDay(c echo.Context) error {
	userID, ok := auth.UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, httpx.MsgUnauthorized)
	}
	trainerID, err := uuid.Parse(c.Param("trainer_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidID)
	}
	var body ClientCompletionRequest
	if err := c.Bind(&body); err != nil {
		return bindError(err)
	}
	if msg := validateCompletionRequest(body); msg != "" {
		return echo.NewHTTPError(http.StatusBadRequest, msg)
	}
	resp, err := h.clientApp.CompleteClientDay(c.Request().Context(), userID, trainerID, body)
	if err != nil {
		return clientAppError(err)
	}
	return c.JSON(http.StatusCreated, resp)
}

func validateCompletionRequest(req ClientCompletionRequest) string {
	switch {
	case req.ProgramAssignmentID == uuid.Nil:
		return "program_assignment_id required"
	case req.ProgramVersionID == uuid.Nil:
		return "program_version_id required"
	case req.CompletionCycleID == uuid.Nil:
		return "completion_cycle_id required"
	case req.DayKey == uuid.Nil:
		return "day_key required"
	}
	text := strings.TrimSpace(req.ResultText)
	if text == "" {
		return "result_text required"
	}
	if hasControlCharacter(req.ResultText) {
		return "result_text contains control characters"
	}
	if utf8.RuneCountInString(text) > workoutcompletion.MaxResultTextLen {
		return "result_text too long"
	}
	return ""
}

// hasControlCharacter reports a C0 control character other than a line break or a tab.
// Postgres refuses a NUL in text, and the others have no use in a workout note.
func hasControlCharacter(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return true
		}
	}
	return false
}

// bindError is the answer for a body that did not decode. A body over the route's size
// limit is 413, also when the reader hit the limit while decoding, which the binder wraps
// as a 400.
func bindError(err error) *echo.HTTPError {
	for e := error(err); e != nil; e = errors.Unwrap(e) {
		if he, ok := e.(*echo.HTTPError); ok && he.Code == http.StatusRequestEntityTooLarge {
			return he
		}
	}
	return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
}

// clientAppError turns a service error into its response and keeps the cause of an
// unexpected one for the request log; the body never carries it.
func clientAppError(err error) *echo.HTTPError {
	he := HTTPErrorFrom(err)
	if he.Code == http.StatusInternalServerError {
		return he.SetInternal(err)
	}
	return he
}
