package subscription

import (
	"net/http"

	"github.com/labstack/echo/v4"

	httpx "mentorix-backend/internal/http"
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

// QuotaErrorBody is the JSON body of a quota refusal. It carries the same code
// and message as every other error body next to the quota details.
type QuotaErrorBody struct {
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Error    string   `json:"error"`
	Resource Resource `json:"resource"`
	Plan     Plan     `json:"plan"`
	Limit    int      `json:"limit"`
	Usage    int      `json:"usage"`
}

// QuotaHTTPError renders the machine-readable 409 quota payload.
func QuotaHTTPError(qe *QuotaError) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusConflict, QuotaErrorBody{
		Code:     httpx.CodeQuotaExceeded,
		Message:  "quota exceeded",
		Error:    httpx.CodeQuotaExceeded,
		Resource: qe.Resource,
		Plan:     qe.Plan,
		Limit:    qe.Limit,
		Usage:    qe.Usage,
	})
}
