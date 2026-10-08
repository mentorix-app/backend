package auth

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"mentorix-backend/internal/config"
	httpx "mentorix-backend/internal/http"
	"mentorix-backend/internal/subscription"
)

type credentialService interface {
	RegisterTrainer(ctx context.Context, email, password, name string) (IssuedAuth, error)
	Login(ctx context.Context, email, password string) (IssuedAuth, error)
	SignInWithGoogle(ctx context.Context, rawIDToken string) (IssuedAuth, error)
	SignInWithApple(ctx context.Context, rawIDToken, name string) (IssuedAuth, error)
	Refresh(ctx context.Context, refreshPlain string) (IssuedAuth, error)
	Logout(ctx context.Context, refreshPlain string) error
	LogoutAll(ctx context.Context, userID uuid.UUID) error
	LinkTelegram(ctx context.Context, appUserID uuid.UUID, code string) (IssuedAuth, error)
	UserProfile(ctx context.Context, userID uuid.UUID) (UserProfile, error)
	UpdateProfileName(ctx context.Context, userID uuid.UUID, name string) (UserProfile, error)
}

// SubscriptionProvider resolves the trainer subscription block for /auth/me.
type SubscriptionProvider interface {
	ForUser(ctx context.Context, userID uuid.UUID) (*subscription.Subscription, error)
}

type Handlers struct {
	svc        credentialService
	jwtSecret  string
	cookie     config.RefreshCookieSettings
	refreshTTL time.Duration
	limiter    *RateLimiter
	subs       SubscriptionProvider
}

type HandlersOption func(*Handlers)

// WithSubscriptions enables the subscription block in /auth/me responses.
func WithSubscriptions(p SubscriptionProvider) HandlersOption {
	return func(h *Handlers) { h.subs = p }
}

func NewHandlers(svc *Service, jwtSecret string, cookie config.RefreshCookieSettings, refreshTTL time.Duration, limiter *RateLimiter, opts ...HandlersOption) *Handlers {
	h := &Handlers{
		svc:        svc,
		jwtSecret:  jwtSecret,
		cookie:     cookie,
		refreshTTL: refreshTTL,
		limiter:    limiter,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Handlers) Mount(e *echo.Echo) {
	e.POST("/auth/register", h.Register)
	e.POST("/auth/login", h.Login)
	e.POST("/auth/google", h.Google)
	e.POST("/auth/apple", h.Apple)
	e.POST("/auth/refresh", h.Refresh)
	e.POST("/auth/logout", h.Logout)
	// Per-route middleware: an empty-prefix Group would add a catch-all that answers 401 for unknown paths.
	jwt := JWTMiddleware(h.jwtSecret)
	e.GET("/auth/me", h.Me, jwt)
	e.PATCH("/auth/me", h.UpdateMe, jwt)
	e.POST("/auth/logout-all", h.LogoutAll, jwt)
	e.POST("/auth/telegram/link", h.TelegramLink, jwt)
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
	return c.JSON(status, tokenResponse(issued))
}

// writeAuthBodyJSON returns the refresh token in the JSON body and leaves cookies alone.
func (h *Handlers) writeAuthBodyJSON(c echo.Context, status int, issued IssuedAuth) error {
	resp := tokenResponse(issued)
	resp.RefreshToken = issued.RefreshToken
	return c.JSON(status, resp)
}

func tokenResponse(issued IssuedAuth) TokenResponse {
	return TokenResponse{
		AccessToken: issued.AccessToken,
		TokenType:   TokenTypeBearer,
		ExpiresAt:   issued.AccessExpires.UTC(),
		UserID:      issued.UserID.String(),
		Email:       issued.Email,
	}
}

func (h *Handlers) Register(c echo.Context) error {
	if err := h.limiter.AllowRegister(c.Request().Context(), c.RealIP()); err != nil {
		return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
	}
	var body RegisterRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	addr, err := mail.ParseAddress(body.Email)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email")
	}
	email := NormalizeEmail(addr.Address)
	if email == "" || utf8.RuneCountInString(email) > maxEmailLen {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid email")
	}
	if err := ValidatePassword(body.Password); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := ValidateDisplayName(body.Name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	issued, err := h.svc.RegisterTrainer(c.Request().Context(), email, body.Password, body.Name)
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
		return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
	}
	var body AuthCredentials
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

func (h *Handlers) Google(c echo.Context) error {
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
	}
	var body IDTokenRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if body.IDToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id_token is required")
	}
	issued, err := h.svc.SignInWithGoogle(c.Request().Context(), body.IDToken)
	if err != nil {
		switch {
		case errors.Is(err, ErrProviderNotConfigured):
			return echo.NewHTTPError(http.StatusServiceUnavailable, "google sign-in is not configured")
		case errors.Is(err, ErrInvalidIDToken):
			return echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidIDToken.Error()).SetInternal(err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "google sign-in failed").SetInternal(err)
	}
	return h.writeAuthBodyJSON(c, http.StatusOK, issued)
}

func (h *Handlers) Apple(c echo.Context) error {
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
	}
	var body AppleSignInRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if body.IDToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id_token is required")
	}
	issued, err := h.svc.SignInWithApple(c.Request().Context(), body.IDToken, body.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrProviderNotConfigured):
			return echo.NewHTTPError(http.StatusServiceUnavailable, "apple sign-in is not configured")
		case errors.Is(err, ErrInvalidIDToken):
			return echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidIDToken.Error()).SetInternal(err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "apple sign-in failed").SetInternal(err)
	}
	return h.writeAuthBodyJSON(c, http.StatusOK, issued)
}

// TelegramLink redeems the bot's one-time code for the signed-in user. The
// answer carries tokens in the body for the account that remains, which may not
// be the caller's own.
func (h *Handlers) TelegramLink(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
	}
	var body TelegramLinkRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	issued, err := h.svc.LinkTelegram(c.Request().Context(), uid, body.Code)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidLinkCode):
			return echo.NewHTTPError(http.StatusBadRequest, ErrInvalidLinkCode.Error())
		case errors.Is(err, ErrLinkCodesUnavailable):
			return echo.NewHTTPError(http.StatusServiceUnavailable, "telegram linking is unavailable").SetInternal(err)
		case errors.Is(err, pgx.ErrNoRows):
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgUserNotFound)
		case errors.Is(err, ErrTelegramLinkConflict):
			return echo.NewHTTPError(http.StatusConflict, ErrTelegramLinkConflict.Error()).SetInternal(err)
		case errors.Is(err, ErrTelegramAlreadyLinked):
			return echo.NewHTTPError(http.StatusConflict, ErrTelegramAlreadyLinked.Error()).SetInternal(err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "telegram link failed").SetInternal(err)
	}
	return h.writeAuthBodyJSON(c, http.StatusOK, issued)
}

func (h *Handlers) Refresh(c echo.Context) error {
	if err := h.limiter.AllowRefresh(c.Request().Context(), c.RealIP()); err != nil {
		return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
	}
	bodyToken := refreshFromBody(c)
	inBody := bodyToken != ""
	plain := bodyToken
	if !inBody {
		plain = h.refreshFromRequest(c)
	}
	if plain == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing refresh token")
	}
	issued, err := h.svc.Refresh(c.Request().Context(), plain)
	if err != nil {
		if errors.Is(err, ErrInvalidRefresh) {
			if !inBody {
				h.clearRefreshCookie(c)
			}
			return echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidRefresh.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "refresh failed")
	}
	if inBody {
		return h.writeAuthBodyJSON(c, http.StatusOK, issued)
	}
	return h.writeAuthJSON(c, http.StatusOK, issued)
}

// refreshFromBody reads the optional refresh token from a JSON body. A request
// without a JSON content type, without a body, or with a body that cannot be
// read yields "" and uses the cookie.
func refreshFromBody(c echo.Context) string {
	if !strings.HasPrefix(c.Request().Header.Get(echo.HeaderContentType), echo.MIMEApplicationJSON) {
		return ""
	}
	var body RefreshRequest
	if err := c.Bind(&body); err != nil {
		return ""
	}
	return body.RefreshToken
}

func (h *Handlers) refreshFromRequest(c echo.Context) string {
	cc, err := c.Cookie(h.cookie.Name)
	if err != nil || cc == nil {
		return ""
	}
	return cc.Value
}

func (h *Handlers) Logout(c echo.Context) error {
	if bodyToken := refreshFromBody(c); bodyToken != "" {
		if err := h.svc.Logout(c.Request().Context(), bodyToken); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "logout failed")
		}
		return c.NoContent(http.StatusNoContent)
	}
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
	return h.writeMeJSON(c, uid, profile)
}

func (h *Handlers) UpdateMe(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	var body MePatchRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if err := ValidateDisplayName(body.Name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	profile, err := h.svc.UpdateProfileName(c.Request().Context(), uid, body.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgUserNotFound)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	return h.writeMeJSON(c, uid, profile)
}

func (h *Handlers) writeMeJSON(c echo.Context, uid uuid.UUID, profile UserProfile) error {
	resp := meResponse(uid, profile)
	if h.subs != nil {
		sub, err := h.subs.ForUser(c.Request().Context(), uid)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
		}
		resp.Subscription = sub
	}
	return c.JSON(http.StatusOK, resp)
}

func meResponse(uid uuid.UUID, profile UserProfile) MeResponse {
	return MeResponse{
		UserID:         uid.String(),
		Email:          profile.Email,
		Name:           profile.Name,
		CreatedAt:      profile.CreatedAt,
		Roles:          profile.Roles,
		TelegramLinked: profile.TelegramLinked,
	}
}
