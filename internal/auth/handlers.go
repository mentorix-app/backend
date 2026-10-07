package auth

import (
	"context"
	"encoding/json"
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
	Refresh(ctx context.Context, refreshPlain string) (IssuedAuth, error)
	SocialLogin(ctx context.Context, provider, idToken, name string) (IssuedAuth, bool, error)
	AddRole(ctx context.Context, userID uuid.UUID, role string) error
	AttachIdentity(ctx context.Context, userID uuid.UUID, provider, idToken, currentPassword string) error
	Logout(ctx context.Context, refreshPlain string) error
	LogoutAll(ctx context.Context, userID uuid.UUID) error
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

	allowedOrigins []string
}

type HandlersOption func(*Handlers)

// WithSubscriptions enables the subscription block in /auth/me responses.
func WithSubscriptions(p SubscriptionProvider) HandlersOption {
	return func(h *Handlers) { h.subs = p }
}

// WithAllowedOrigins sets the origins allowed to send the refresh cookie to
// refresh and logout. An empty list or a list containing "*" disables the check.
func WithAllowedOrigins(origins []string) HandlersOption {
	return func(h *Handlers) { h.allowedOrigins = origins }
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
	e.POST("/auth/social-login", h.SocialLogin)
	e.POST("/auth/refresh", h.Refresh)
	e.POST("/auth/logout", h.Logout)
	g := e.Group("", JWTMiddleware(h.jwtSecret))
	g.GET("/auth/me", h.Me)
	g.PATCH("/auth/me", h.UpdateMe)
	g.POST("/auth/me/roles", h.AddRole)
	g.POST("/auth/me/identities", h.AttachIdentity)
	g.POST("/auth/logout-all", h.LogoutAll)
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

// writeAuthJSON sends the refresh token in the JSON body when delivery is
// TokenDeliveryBody and in the refresh cookie otherwise.
func (h *Handlers) writeAuthJSON(c echo.Context, status int, issued IssuedAuth, delivery string) error {
	resp := TokenResponse{
		AccessToken: issued.AccessToken,
		TokenType:   TokenTypeBearer,
		ExpiresAt:   issued.AccessExpires.UTC(),
		UserID:      issued.UserID.String(),
		Email:       issued.Email,
	}
	if delivery == TokenDeliveryBody {
		resp.RefreshToken = issued.RefreshToken
	} else {
		h.setRefreshCookie(c, issued.RefreshToken)
	}
	c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
	return c.JSON(status, resp)
}

func validTokenDelivery(v string) bool {
	return v == "" || v == TokenDeliveryCookie || v == TokenDeliveryBody
}

func (h *Handlers) Register(c echo.Context) error {
	if err := h.limiter.AllowRegister(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	var body RegisterRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if !validTokenDelivery(body.TokenDelivery) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid token_delivery")
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
	issued, err := h.svc.RegisterTrainer(c.Request().Context(), email, body.Password, body.Name)
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return echo.NewHTTPError(http.StatusConflict, ErrEmailTaken.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "registration failed")
	}
	return h.writeAuthJSON(c, http.StatusCreated, issued, body.TokenDelivery)
}

func (h *Handlers) Login(c echo.Context) error {
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	var body AuthCredentials
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if !validTokenDelivery(body.TokenDelivery) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid token_delivery")
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
	return h.writeAuthJSON(c, http.StatusOK, issued, body.TokenDelivery)
}

// SocialLogin signs in with an Apple or Google ID token. It returns 201 when the
// token's identity had no account yet and 200 otherwise.
func (h *Handlers) SocialLogin(c echo.Context) error {
	var body SocialLoginRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if body.Provider != ProviderApple && body.Provider != ProviderGoogle {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid provider")
	}
	idToken := strings.TrimSpace(body.IDToken)
	if idToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id_token is required")
	}
	if !validTokenDelivery(body.TokenDelivery) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid token_delivery")
	}
	if !usableName(body.Name) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid name")
	}
	if utf8.RuneCountInString(normalizeDisplayName(body.Name)) > maxSocialNameRunes {
		return echo.NewHTTPError(http.StatusBadRequest, "name is too long")
	}
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	issued, created, err := h.svc.SocialLogin(c.Request().Context(), body.Provider, idToken, body.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidIDToken):
			return echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidIDToken.Error())
		case errors.Is(err, ErrIDTokenProviderUnavailable):
			// The detail goes to the request log through Internal, not to the client.
			return echo.NewHTTPError(http.StatusServiceUnavailable, "sign-in provider unavailable").SetInternal(err)
		case errors.Is(err, ErrProviderNotConfigured):
			return echo.NewHTTPError(http.StatusBadRequest, ErrProviderNotConfigured.Error())
		case errors.Is(err, ErrEmailBelongsToAnotherAccount):
			return echo.NewHTTPError(http.StatusConflict, "an account with this email already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "social login failed")
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return h.writeAuthJSON(c, status, issued, body.TokenDelivery)
}

func (h *Handlers) Refresh(c echo.Context) error {
	bodyToken, cookieToken := h.refreshTokens(c)
	fromBody := bodyToken != ""
	plain := cookieToken
	if fromBody {
		plain = bodyToken
	}
	// Rejected cross-site requests return before the rate limiter so they cannot
	// spend the victim's budget.
	if cookieToken != "" && !fromBody && !h.originAllowed(c) {
		return echo.NewHTTPError(http.StatusForbidden, msgOriginNotAllowed)
	}
	if err := h.limiter.AllowRefresh(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	if plain == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing refresh token")
	}
	issued, err := h.svc.Refresh(c.Request().Context(), plain)
	if err != nil {
		if errors.Is(err, ErrRefreshBusy) {
			return echo.NewHTTPError(http.StatusServiceUnavailable, msgTryAgain).SetInternal(err)
		}
		if errors.Is(err, ErrInvalidRefresh) {
			if !fromBody {
				h.clearRefreshCookie(c)
			}
			httpErr := echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidRefresh.Error())
			if errors.Is(err, ErrRefreshTokenReused) {
				// The client sees an ordinary 401; the request log gets the cause and family id.
				return httpErr.SetInternal(err)
			}
			return httpErr
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "refresh failed")
	}
	// The response mode follows the token's origin: a cookie refresh never
	// returns the new refresh token in JSON.
	delivery := TokenDeliveryCookie
	if fromBody {
		delivery = TokenDeliveryBody
	}
	return h.writeAuthJSON(c, http.StatusOK, issued, delivery)
}

const (
	msgOriginNotAllowed = "origin not allowed"
	msgTryAgain         = "session busy, try again"
)

// OriginCheckDisabled reports whether the refresh cookie Origin check is off
// for the configured origins: an empty list or a list containing "*".
func OriginCheckDisabled(origins []string) bool {
	if len(origins) == 0 {
		return true
	}
	for _, o := range origins {
		if strings.TrimSpace(o) == "*" {
			return true
		}
	}
	return false
}

// originAllowed guards requests that act on the refresh cookie. With SameSite=None
// another site can make the browser send the cookie, so a request whose Origin
// is outside the configured list is refused. A missing Origin header or a
// disabled check (see OriginCheckDisabled) allows the request.
func (h *Handlers) originAllowed(c echo.Context) bool {
	origin := c.Request().Header.Get(echo.HeaderOrigin)
	if origin == "" || OriginCheckDisabled(h.allowedOrigins) {
		return true
	}
	for _, allowed := range h.allowedOrigins {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(allowed), "/"), origin) {
			return true
		}
	}
	return false
}

// refreshTokens returns the refresh token from the optional JSON body and the
// one from the cookie; either may be empty. A body that does not decode, or
// whose token is blank, counts as no body token. The body is decoded directly
// because clients may omit Content-Type.
func (h *Handlers) refreshTokens(c echo.Context) (bodyToken, cookieToken string) {
	var body RefreshRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err == nil {
		bodyToken = strings.TrimSpace(body.RefreshToken)
	}
	if cc, err := c.Cookie(h.cookie.Name); err == nil && cc != nil {
		cookieToken = cc.Value
	}
	return bodyToken, cookieToken
}

// Logout revokes the body token and the cookie token when both are sent, and
// clears the cookie unless only a body token was sent. Both revokes are
// attempted even if the first fails; any failure skips the cookie clear.
func (h *Handlers) Logout(c echo.Context) error {
	bodyToken, cookieToken := h.refreshTokens(c)
	if cookieToken != "" && !h.originAllowed(c) {
		return echo.NewHTTPError(http.StatusForbidden, msgOriginNotAllowed)
	}
	if err := h.limiter.AllowLogout(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	var revokeErr error
	if bodyToken != "" {
		revokeErr = h.svc.Logout(c.Request().Context(), bodyToken)
	}
	if cookieToken != "" && cookieToken != bodyToken {
		revokeErr = errors.Join(revokeErr, h.svc.Logout(c.Request().Context(), cookieToken))
	}
	if revokeErr != nil {
		if errors.Is(revokeErr, ErrRefreshBusy) {
			return echo.NewHTTPError(http.StatusServiceUnavailable, msgTryAgain).SetInternal(revokeErr)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "logout failed")
	}
	if bodyToken == "" || cookieToken != "" {
		h.clearRefreshCookie(c)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handlers) LogoutAll(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	if err := h.svc.LogoutAll(c.Request().Context(), uid); err != nil {
		if errors.Is(err, ErrRefreshBusy) {
			return echo.NewHTTPError(http.StatusServiceUnavailable, msgTryAgain).SetInternal(err)
		}
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
	profile, err := h.svc.UpdateProfileName(c.Request().Context(), uid, body.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgUserNotFound)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	return h.writeMeJSON(c, uid, profile)
}

// AddRole gives the signed-in user the trainer or client role and returns the
// updated profile. Repeating the call is harmless.
func (h *Handlers) AddRole(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	var body AddRoleRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if body.Role != RoleTrainer && body.Role != RoleClient {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid role")
	}
	if err := h.svc.AddRole(c.Request().Context(), uid, body.Role); err != nil {
		if errors.Is(err, ErrRoleConflict) {
			return echo.NewHTTPError(http.StatusConflict, ErrRoleConflict.Error())
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgUserNotFound)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "add role failed")
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

// AttachIdentity links an Apple or Google sign-in to the signed-in account and
// returns the updated profile. Repeating the call is harmless.
func (h *Handlers) AttachIdentity(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	var body AttachIdentityRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	if body.Provider != ProviderApple && body.Provider != ProviderGoogle {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid provider")
	}
	idToken := strings.TrimSpace(body.IDToken)
	if idToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id_token is required")
	}
	if utf8.RuneCountInString(body.CurrentPassword) > maxPasswordLen {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid current_password")
	}
	if err := h.limiter.AllowAttachIdentity(c.Request().Context(), c.RealIP()); err != nil {
		if errors.Is(err, ErrRateLimited) {
			return echo.NewHTTPError(http.StatusTooManyRequests, ErrRateLimited.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "rate limit failed")
	}
	if err := h.svc.AttachIdentity(c.Request().Context(), uid, body.Provider, idToken, body.CurrentPassword); err != nil {
		switch {
		case errors.Is(err, ErrInvalidIDToken):
			return echo.NewHTTPError(http.StatusUnauthorized, ErrInvalidIDToken.Error())
		case errors.Is(err, ErrIDTokenProviderUnavailable):
			return echo.NewHTTPError(http.StatusServiceUnavailable, "sign-in provider unavailable").SetInternal(err)
		case errors.Is(err, ErrProviderNotConfigured):
			return echo.NewHTTPError(http.StatusBadRequest, ErrProviderNotConfigured.Error())
		case errors.Is(err, ErrAdminCannotAttachIdentity):
			return echo.NewHTTPError(http.StatusForbidden, ErrAdminCannotAttachIdentity.Error())
		case errors.Is(err, ErrPasswordRequired):
			return echo.NewHTTPError(http.StatusForbidden, ErrPasswordRequired.Error())
		case errors.Is(err, ErrPasswordIncorrect):
			return echo.NewHTTPError(http.StatusForbidden, ErrPasswordIncorrect.Error())
		case errors.Is(err, ErrIdentityBelongsToAnotherAccount):
			return echo.NewHTTPError(http.StatusConflict, "this sign-in already belongs to another account")
		case errors.Is(err, pgx.ErrNoRows):
			return echo.NewHTTPError(http.StatusNotFound, httpx.MsgUserNotFound)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "attach identity failed")
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
	roles := profile.Roles
	if roles == nil {
		roles = []string{}
	}
	methods := profile.SignInMethods
	if methods == nil {
		methods = []string{}
	}
	return MeResponse{
		UserID:        uid.String(),
		Email:         profile.Email,
		Name:          profile.Name,
		CreatedAt:     profile.CreatedAt,
		Roles:         roles,
		SignInMethods: methods,
	}
}
