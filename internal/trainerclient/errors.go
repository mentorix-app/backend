package trainerclient

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
	"mentorix-backend/internal/workoutcompletion"
)

func HTTPErrorFrom(err error) *echo.HTTPError {
	var qe *subscription.QuotaError
	if errors.As(err, &qe) {
		return subscription.QuotaHTTPError(qe)
	}
	switch {
	case errors.Is(err, ErrClientLimitReached):
		return httpx.NewError(http.StatusConflict, httpx.CodeClientLimitReached, err.Error())
	case errors.Is(err, ErrSelfInvite):
		return httpx.NewError(http.StatusUnprocessableEntity, httpx.CodeSelfInvite, err.Error())
	case errors.Is(err, ErrInviteNotFound):
		return httpx.NewError(http.StatusNotFound, httpx.CodeInviteNotFound, err.Error())
	case errors.Is(err, ErrInviteExpired):
		return httpx.NewError(http.StatusGone, httpx.CodeInviteExpired, err.Error())
	case errors.Is(err, ErrInviteConsumed):
		return httpx.NewError(http.StatusConflict, httpx.CodeInviteConsumed, err.Error())
	case errors.Is(err, ErrInviteNotConfigured):
		return httpx.NewError(http.StatusServiceUnavailable, httpx.CodeInviteNotConfigured, err.Error())
	case errors.Is(err, program.ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, program.ErrClientNotLinked):
		return httpx.NewError(http.StatusForbidden, httpx.CodeClientNotLinked, err.Error())
	case errors.Is(err, ErrTrainerNotLinked):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, ErrAdminCannotAccept):
		return httpx.NewError(http.StatusForbidden, httpx.CodeAdminCannotAccept, err.Error())
	case errors.Is(err, ErrActiveTrainerNotSet):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, ErrTelegramUserNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, ErrUserNotFound):
		return httpx.NewError(http.StatusNotFound, httpx.CodeUserNotFound, err.Error())
	case errors.Is(err, program.ErrClientBlocked):
		return httpx.NewError(http.StatusUnprocessableEntity, httpx.CodeClientBlocked, err.Error())
	case errors.Is(err, program.ErrProgramNotPublished):
		return httpx.NewError(http.StatusUnprocessableEntity, httpx.CodeProgramNotPublished, err.Error())
	case errors.Is(err, program.ErrClientNotFound):
		return httpx.NewError(http.StatusNotFound, httpx.CodeClientNotFound, err.Error())
	case errors.Is(err, program.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, ErrProgramChanged):
		return httpx.NewError(http.StatusConflict, httpx.CodeProgramChanged, err.Error())
	case errors.Is(err, ErrDayNotFound):
		return httpx.NewError(http.StatusNotFound, httpx.CodeDayNotFound, err.Error())
	case errors.Is(err, workoutcompletion.ErrAlreadyCompleted):
		return httpx.NewError(http.StatusConflict, httpx.CodeWorkoutAlreadyCompleted, err.Error())
	case errors.Is(err, workoutcompletion.ErrValidation):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "internal")
	}
}
