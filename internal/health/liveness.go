package health

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func RegisterLiveness(e *echo.Echo) {
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": StatusOK})
	})
}
