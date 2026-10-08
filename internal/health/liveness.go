package health

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// LivenessResponse is the GET /health body. Commit is omitted when unknown.
type LivenessResponse struct {
	Status string `json:"status"`
	Commit string `json:"commit,omitempty"`
}

// RegisterLiveness mounts GET /health. commit is the deployed git commit, or
// empty to leave it out of the response.
func RegisterLiveness(e *echo.Echo, commit string) {
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, LivenessResponse{Status: StatusOK, Commit: commit})
	})
}
