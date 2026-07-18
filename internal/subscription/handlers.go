package subscription

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Handlers serves the public plan catalog.
type Handlers struct {
	authMW echo.MiddlewareFunc
}

// NewHandlers takes the JWT middleware to avoid an import cycle with auth.
func NewHandlers(authMW echo.MiddlewareFunc) *Handlers {
	return &Handlers{authMW: authMW}
}

func (h *Handlers) Mount(e *echo.Echo) {
	e.GET("/plans", h.ListPlans, h.authMW)
}

type planListResponse struct {
	Items []PlanCatalogItem `json:"items"`
}

func (h *Handlers) ListPlans(c echo.Context) error {
	return c.JSON(http.StatusOK, planListResponse{Items: Catalog()})
}

type quotaErrorBody struct {
	Error    string   `json:"error"`
	Resource Resource `json:"resource"`
	Plan     Plan     `json:"plan"`
	Limit    int      `json:"limit"`
	Usage    int      `json:"usage"`
}

// QuotaHTTPError renders the machine-readable 409 quota payload.
func QuotaHTTPError(qe *QuotaError) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusConflict, quotaErrorBody{
		Error:    "quota_exceeded",
		Resource: qe.Resource,
		Plan:     qe.Plan,
		Limit:    qe.Limit,
		Usage:    qe.Usage,
	})
}
