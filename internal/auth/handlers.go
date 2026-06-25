package auth

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/config"
	httpx "mentorix-backend/internal/http"
)

type credentialService interface {
	RegisterTrainer(ctx context.Context, email, password string) (IssuedAuth, error)
	Login(ctx context.Context, email, password string) (IssuedAuth, error)
	Refresh(ctx context.Context, refreshPlain string) (IssuedAuth, error)
	Logout(ctx context.Context, refreshPlain string) error
	LogoutAll(ctx context.Context, userID uuid.UUID) error
	UserProfile(ctx context.Context, userID uuid.UUID) (UserProfile, error)
}

type Handlers struct {
	svc        credentialService
	jwtSecret  string
	cookie     config.RefreshCookieSettings
	refreshTTL time.Duration
	limiter    *RateLimiter
}

func NewHandlers(svc *Service, jwtSecret string, cookie config.RefreshCookieSettings, refreshTTL time.Duration, limiter *RateLimiter) *Handlers {
	return &Handlers{
		svc:        svc,
		jwtSecret:  jwtSecret,
		cookie:     cookie,
		refreshTTL: refreshTTL,
		limiter:    limiter,
	}
}

func (h *Handlers) Mount(e *echo.Echo) {
	e.POST("/auth/register", h.Register)
	e.POST("/auth/login", h.Login)
	e.POST("/auth/refresh", h.Refresh)
	e.POST("/auth/logout", h.Logout)
	g := e.Group("", JWTMiddleware(h.jwtSecret))
	g.GET("/auth/me", h.Me)
	g.POST("/auth/logout-all", h.LogoutAll)
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
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	Roles     []string  `json:"roles"`
}

func (h *Handlers) setRefreshCookie(c echo.Context, value string) {
	ck := &http.Cookie{
		Name:     h.cookie.Name,
		Value:    value,
		Path:     h.cookie.Path,
		Domain:   h.cookie.Domain,
		MaxAge:   int(h.refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cookie.Secure,
		SameSite: h.cookie.SameSite,
	}
	c.SetCookie(ck)
}

func (h *Handlers) clearRefreshCookie(c echo.Context) {
	ck := &http.Cookie{
		Name:     h.cookie.Name,
		Value:    "",
		Path:     h.cookie.Path,
		Domain:   h.cookie.Domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookie.Secure,
		SameSite: h.cookie.SameSite,
	}
	c.SetCookie(ck)
}

func (h *Handlers) writeAuthJSON(c echo.Context, status int, issued IssuedAuth) error {
	h.setRefreshCookie(c, issued.RefreshToken)
	return c.JSON(status, tokenResponse{
		AccessToken: issued.AccessToken,
		TokenType:   TokenTypeBearer,
		ExpiresAt:   issued.AccessExpires.UTC(),
		UserID:      issued.UserID.String(),
		Email:       issued.Email,
	})
}

func (h *Handlers) Register(c echo.Context) error {
	if err := h.limiter.AllowRegister(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	var body authCredentialsBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
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
	issued, err := h.svc.RegisterTrainer(c.Request().Context(), email, body.Password)
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return echo.NewHTTPError(http.StatusConflict, ErrEmailTaken.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "registration failed")
	}
	return h.writeAuthJSON(c, http.StatusCreated, issued)
}

func (h *Handlers) Login(c echo.Context) error {
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	var body authCredentialsBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	addr, err := mail.ParseAddress(body.Email)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidCredentials.Error())
	}
	email := NormalizeEmail(addr.Address)
	issued, err := h.svc.Login(c.Request().Context(), email, body.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidCredentials.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "login failed")
	}
	return h.writeAuthJSON(c, http.StatusOK, issued)
}

func (h *Handlers) Refresh(c echo.Context) error {
	if err := h.limiter.AllowRefresh(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	plain := h.refreshFromRequest(c)
	if plain == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing refresh token")
	}
	issued, err := h.svc.Refresh(c.Request().Context(), plain)
	if err != nil {
		if errors.Is(err, ErrInvalidRefresh) {
			h.clearRefreshCookie(c)
			return echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidRefresh.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "refresh failed")
	}
	return h.writeAuthJSON(c, http.StatusOK, issued)
}

func (h *Handlers) refreshFromRequest(c echo.Context) string {
	cc, err := c.Cookie(h.cookie.Name)
	if err != nil || cc == nil {
		return ""
	}
	return cc.Value
}

func (h *Handlers) Logout(c echo.Context) error {
	plain := h.refreshFromRequest(c)
	if plain != "" {
		if err := h.svc.Logout(c.Request().Context(), plain); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "logout failed")
		}
	}
	h.clearRefreshCookie(c)
	return c.NoContent(http.StatusNoContent)
}

func (h *Handlers) LogoutAll(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	if err := h.svc.LogoutAll(c.Request().Context(), uid); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "logout failed")
	}
	h.clearRefreshCookie(c)
	return c.NoContent(http.StatusNoContent)
}

func (h *Handlers) Me(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	profile, err := h.svc.UserProfile(c.Request().Context(), uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgUserNotFound)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	return c.JSON(http.StatusOK, meResponse{
		UserID:    uid.String(),
		Email:     profile.Email,
		CreatedAt: profile.CreatedAt,
		Roles:     profile.Roles,
	})
}
