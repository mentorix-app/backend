package auth

import (
	"errors"
	"net/http"
	"net/mail"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

type Handlers struct {
	svc       *Service
	jwtSecret string
}

func NewHandlers(svc *Service, jwtSecret string) *Handlers {
	return &Handlers{svc: svc, jwtSecret: jwtSecret}
}

func (h *Handlers) Mount(e *echo.Echo) {
	e.POST("/auth/register", h.Register)
	e.POST("/auth/login", h.Login)
	g := e.Group("", JWTMiddleware(h.jwtSecret))
	g.GET("/auth/me", h.Me)
}

type authCredentialsBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
}

type meResponse struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

func (h *Handlers) Register(c echo.Context) error {
	var body authCredentialsBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid json")
	}
	addr, err := mail.ParseAddress(body.Email)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email")
	}
	email := NormalizeEmail(addr.Address)
	if email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email")
	}
	if err := ValidatePassword(body.Password); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	token, exp, userID, err := h.svc.RegisterTrainer(c.Request().Context(), email, body.Password)
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return echo.NewHTTPError(http.StatusConflict, "email already registered")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "registration failed")
	}
	return c.JSON(http.StatusCreated, tokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   exp.UTC(),
		UserID:      userID.String(),
		Email:       email,
	})
}

func (h *Handlers) Login(c echo.Context) error {
	var body authCredentialsBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid json")
	}
	addr, err := mail.ParseAddress(body.Email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid email or password")
	}
	email := NormalizeEmail(addr.Address)
	token, exp, userID, err := h.svc.Login(c.Request().Context(), email, body.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid email or password")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "login failed")
	}
	return c.JSON(http.StatusOK, tokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   exp.UTC(),
		UserID:      userID.String(),
		Email:       email,
	})
}

func (h *Handlers) Me(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal")
	}
	email, err := h.svc.UserPrimaryEmail(c.Request().Context(), uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal")
	}
	return c.JSON(http.StatusOK, meResponse{UserID: uid.String(), Email: email})
}
