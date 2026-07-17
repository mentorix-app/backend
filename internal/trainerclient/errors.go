package trainerclient

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/subscription"
)

func HTTPErrorFrom(err error) *echo.HTTPError {
	var qe *subscription.QuotaError
	if errors.As(err, &qe) {
		return subscription.QuotaHTTPError(qe)
	}
	switch {
	case errors.Is(err, ErrClientLimitReached):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, ErrSelfInvite):
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrInviteNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInviteExpired):
		return echo.NewHTTPError(http.StatusGone, err.Error())
	case errors.Is(err, ErrInviteConsumed):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, ErrInviteNotConfigured):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, program.ErrForbidden), errors.Is(err, program.ErrClientNotLinked), errors.Is(err, ErrTrainerNotLinked):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, ErrActiveTrainerNotSet):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, ErrTelegramUserNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, program.ErrClientBlocked), errors.Is(err, program.ErrProgramNotPublished):
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, program.ErrClientNotFound), errors.Is(err, program.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "internal")
	}
}
