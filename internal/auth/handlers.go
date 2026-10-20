package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

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
	DeleteAccount(ctx context.Context, userID uuid.UUID, proof AppleProof) error
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

	appleAndroidPackage string
}

type HandlersOption func(*Handlers)

// WithSubscriptions enables the subscription block in /auth/me responses.
func WithSubscriptions(p SubscriptionProvider) HandlersOption {
	return func(h *Handlers) { h.subs = p }
}

// WithAppleAndroidPackage sets the Android application id that POST
// /auth/apple/callback sends the browser back to. Without it the callback answers 503.
func WithAppleAndroidPackage(pkg string) HandlersOption {
	return func(h *Handlers) { h.appleAndroidPackage = pkg }
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
	e.POST("/auth/apple/callback", h.AppleCallback, middleware.BodyLimit("16K"))
	e.POST("/auth/refresh", h.Refresh)
	e.POST("/auth/logout", h.Logout)
	// Per-route middleware: an empty-prefix Group would add a catch-all that answers 401 for unknown paths.
	jwt := JWTMiddleware(h.jwtSecret)
	e.GET("/auth/me", h.Me, jwt)
	e.PATCH("/auth/me", h.UpdateMe, jwt)
	e.DELETE("/auth/me", h.DeleteMe, jwt, middleware.BodyLimit(strconv.Itoa(deleteAccountBodyLimit)))
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
		AccessToken:      issued.AccessToken,
		TokenType:        TokenTypeBearer,
		ExpiresAt:        issued.AccessExpires.UTC(),
		RefreshExpiresAt: issued.RefreshExpires.UTC(),
		UserID:           issued.UserID.String(),
		Email:            issued.Email,
	}
}

// rateLimited answers 429 with Retry-After set from the limiter's error.
func rateLimited(c echo.Context, err error) error {
	var rl *RateLimitError
	var wait time.Duration
	if errors.As(err, &rl) {
		wait = rl.RetryAfter
	}
	c.Response().Header().Set(echo.HeaderRetryAfter, retryAfterSeconds(wait))
	return httpx.NewError(http.StatusTooManyRequests, httpx.CodeRateLimited, ErrRateLimited.Error())
}

// retryAfterSeconds rounds up to whole seconds and never answers 0.
func retryAfterSeconds(d time.Duration) string {
	secs := int64((d + time.Second - 1) / time.Second)
	return strconv.FormatInt(max(secs, 1), 10)
}

func (h *Handlers) Register(c echo.Context) error {
	if err := h.limiter.AllowRegister(c.Request().Context(), c.RealIP()); err != nil {
		return rateLimited(c, err)
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
			return httpx.NewError(http.StatusConflict, httpx.CodeEmailTaken, ErrEmailTaken.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "registration failed")
	}
	return h.writeAuthJSON(c, http.StatusCreated, issued)
}

func (h *Handlers) Login(c echo.Context) error {
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		return rateLimited(c, err)
	}
	var body AuthCredentials
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	addr, err := mail.ParseAddress(body.Email)
	if err != nil {
		return httpx.NewError(http.StatusUnauthorized, httpx.CodeInvalidCredentials, ErrInvalidCredentials.Error())
	}
	email := NormalizeEmail(addr.Address)
	issued, err := h.svc.Login(c.Request().Context(), email, body.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return httpx.NewError(http.StatusUnauthorized, httpx.CodeInvalidCredentials, ErrInvalidCredentials.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "login failed")
	}
	return h.writeAuthJSON(c, http.StatusOK, issued)
}

func (h *Handlers) Google(c echo.Context) error {
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		return rateLimited(c, err)
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
			return httpx.NewError(http.StatusServiceUnavailable, httpx.CodeProviderNotConfigured, "google sign-in is not configured")
		case errors.Is(err, ErrInvalidIDToken):
			return httpx.NewErrorCause(http.StatusUnauthorized, httpx.CodeInvalidIDToken, ErrInvalidIDToken.Error(), err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "google sign-in failed").SetInternal(err)
	}
	return h.writeAuthBodyJSON(c, http.StatusOK, issued)
}

func (h *Handlers) Apple(c echo.Context) error {
	if err := h.limiter.AllowLogin(c.Request().Context(), c.RealIP()); err != nil {
		return rateLimited(c, err)
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
			return httpx.NewError(http.StatusServiceUnavailable, httpx.CodeProviderNotConfigured, "apple sign-in is not configured")
		case errors.Is(err, ErrInvalidIDToken):
			return httpx.NewErrorCause(http.StatusUnauthorized, httpx.CodeInvalidIDToken, ErrInvalidIDToken.Error(), err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "apple sign-in failed").SetInternal(err)
	}
	return h.writeAuthBodyJSON(c, http.StatusOK, issued)
}

// AppleCallback ends Sign in with Apple on Android. Apple makes the browser POST
// the result as a form; the answer sends the browser back into the app with an
// intent link that carries the form fields. It creates no session and checks no
// token: the app sends the id_token to POST /auth/apple afterwards.
func (h *Handlers) AppleCallback(c echo.Context) error {
	if h.appleAndroidPackage == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "apple sign-in return is not configured")
	}
	req := c.Request()
	// ParseForm reads only an application/x-www-form-urlencoded body. For another
	// content type it leaves PostForm empty, and for a Content-Type header it
	// cannot parse it returns an error; both end in 400.
	if err := req.ParseForm(); err != nil {
		// The body limit middleware reports an oversized body as a read error.
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) && httpErr.Code == http.StatusRequestEntityTooLarge {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge)
		}
		// The parse error can quote a fragment of the form, so it stays out of the answer and the logs.
		return echo.NewHTTPError(http.StatusBadRequest, "invalid form body")
	}
	// PostForm only: ParseForm also merges the URL query into Form, and nothing
	// but Apple's form body may reach the app.
	form := req.PostForm
	if len(form) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "form body is required")
	}
	c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
	// Encode escapes '#', ';' and '&', so a field cannot change the part after '#'.
	return c.Redirect(http.StatusTemporaryRedirect,
		"intent://callback?"+form.Encode()+"#Intent;package="+h.appleAndroidPackage+";scheme=signinwithapple;end")
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
		return rateLimited(c, err)
	}
	var body TelegramLinkRequest
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	issued, err := h.svc.LinkTelegram(c.Request().Context(), uid, body.Code)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidLinkCode):
			return httpx.NewError(http.StatusBadRequest, httpx.CodeInvalidLinkCode, ErrInvalidLinkCode.Error())
		case errors.Is(err, ErrLinkCodesUnavailable):
			return httpx.NewErrorCause(http.StatusServiceUnavailable, httpx.CodeTelegramLinkUnavailable, "telegram linking is unavailable", err)
		case errors.Is(err, pgx.ErrNoRows):
			return httpx.NewError(http.StatusNotFound, httpx.CodeUserNotFound, httpx.MsgUserNotFound)
		case errors.Is(err, ErrTelegramLinkConflict):
			return httpx.NewErrorCause(http.StatusConflict, httpx.CodeTelegramLinkConflict, ErrTelegramLinkConflict.Error(), err)
		case errors.Is(err, ErrTelegramAlreadyLinked):
			return httpx.NewErrorCause(http.StatusConflict, httpx.CodeTelegramAlreadyLinked, ErrTelegramAlreadyLinked.Error(), err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "telegram link failed").SetInternal(err)
	}
	return h.writeAuthBodyJSON(c, http.StatusOK, issued)
}

func (h *Handlers) Refresh(c echo.Context) error {
	if err := h.limiter.AllowRefresh(c.Request().Context(), c.RealIP()); err != nil {
		return rateLimited(c, err)
	}
	bodyToken := refreshFromBody(c)
	inBody := bodyToken != ""
	plain := bodyToken
	if !inBody {
		plain = h.refreshFromRequest(c)
	}
	if plain == "" {
		return httpx.NewError(http.StatusUnauthorized, httpx.CodeMissingRefreshToken, "missing refresh token")
	}
	issued, err := h.svc.Refresh(c.Request().Context(), plain)
	if err != nil {
		if errors.Is(err, ErrInvalidRefresh) {
			if !inBody {
				h.clearRefreshCookie(c)
			}
			return httpx.NewError(http.StatusUnauthorized, httpx.CodeInvalidRefreshToken, ErrInvalidRefresh.Error())
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
			return httpx.NewError(http.StatusNotFound, httpx.CodeUserNotFound, httpx.MsgUserNotFound)
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
			return httpx.NewError(http.StatusNotFound, httpx.CodeUserNotFound, httpx.MsgUserNotFound)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	return h.writeMeJSON(c, uid, profile)
}

// DeleteMe deletes the caller's own account and every row that belongs to it. The
// body is optional; an account with an Apple sign-in sends the Apple proof in it. It
// leaves cookies alone: the app clears its own tokens.
func (h *Handlers) DeleteMe(c echo.Context) error {
	uid, ok := UserIDFromContext(c)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, httpx.MsgInternal)
	}
	body, err := readDeleteAccountRequest(c)
	if err != nil {
		return err
	}
	if err := validateDeleteAccountRequest(body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	proof := AppleProof{IDToken: body.AppleIDToken, AuthorizationCode: body.AppleAuthorizationCode, RedirectURI: body.AppleRedirectURI}
	if err := h.svc.DeleteAccount(c.Request().Context(), uid, proof); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return httpx.NewError(http.StatusNotFound, httpx.CodeUserNotFound, httpx.MsgUserNotFound)
		case errors.Is(err, ErrAccountDeletionUnavailable):
			return httpx.NewErrorCause(http.StatusConflict, httpx.CodeAccountDeletionUnavailable, ErrAccountDeletionUnavailable.Error(), err)
		case errors.Is(err, ErrAppleAuthorizationRequired):
			return httpx.NewError(http.StatusUnprocessableEntity, httpx.CodeAppleAuthorizationRequired, ErrAppleAuthorizationRequired.Error())
		case errors.Is(err, ErrInvalidIDToken):
			return httpx.NewErrorCause(http.StatusUnauthorized, httpx.CodeInvalidIDToken, ErrInvalidIDToken.Error(), err)
		case errors.Is(err, ErrAppleAccountMismatch):
			return httpx.NewErrorCause(http.StatusForbidden, httpx.CodeAppleAccountMismatch, ErrAppleAccountMismatch.Error(), err)
		case errors.Is(err, ErrInvalidAppleAuthorizationCode):
			return httpx.NewErrorCause(http.StatusBadRequest, httpx.CodeInvalidAppleAuthorizationCode, ErrInvalidAppleAuthorizationCode.Error(), err)
		case errors.Is(err, ErrAppleRevocationFailed):
			return httpx.NewErrorCause(http.StatusBadGateway, httpx.CodeAppleRevocationFailed, ErrAppleRevocationFailed.Error(), err)
		case errors.Is(err, ErrProviderNotConfigured):
			return httpx.NewError(http.StatusServiceUnavailable, httpx.CodeProviderNotConfigured, "apple account deletion is not configured")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "account deletion failed").SetInternal(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// readDeleteAccountRequest reads the optional JSON body of DELETE /auth/me. A missing or
// blank body is an empty request, whatever its framing and Content-Type; a body that
// has text must carry the Content-Type application/json and be one JSON object.
//
// It does not use Echo's Bind. Bind refuses an empty body of unknown length (a chunked
// request ends in io.EOF), a body of only whitespace, and a mixed-case Content-Type
// such as Application/JSON. And the route's body limit holds exactly only when the body
// is read to its end, as it is here; a streaming decoder can stop before the reader
// reports the excess.
func readDeleteAccountRequest(c echo.Context) (DeleteAccountRequest, error) {
	raw, err := io.ReadAll(c.Request().Body)
	if err != nil {
		// The body limit middleware reports an oversized body as a read error.
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) && httpErr.Code == http.StatusRequestEntityTooLarge {
			return DeleteAccountRequest{}, echo.NewHTTPError(http.StatusRequestEntityTooLarge)
		}
		return DeleteAccountRequest{}, echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return DeleteAccountRequest{}, nil
	}
	// ParseMediaType lowercases the type and sets the parameters, such as charset, aside.
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	var body DeleteAccountRequest
	// The decoder's error can quote a piece of the body, so it stays out of the answer and the logs.
	if err != nil || mediaType != echo.MIMEApplicationJSON || json.Unmarshal(raw, &body) != nil {
		return DeleteAccountRequest{}, echo.NewHTTPError(http.StatusBadRequest, httpx.MsgInvalidJSON)
	}
	return body, nil
}

// deleteAccountBodyLimit is the largest body of DELETE /auth/me, in bytes.
const deleteAccountBodyLimit = 16000

// maxAppleRedirectURILen is the longest apple_redirect_uri, in characters.
const maxAppleRedirectURILen = 2048

// validateDeleteAccountRequest checks that the Apple fields come as a pair, that the
// redirect address needs the pair, and that the address is an absolute https URL
// without a fragment. Which of the fields a given account needs is decided later.
func validateDeleteAccountRequest(b DeleteAccountRequest) error {
	if (b.AppleIDToken == "") != (b.AppleAuthorizationCode == "") {
		return errors.New("apple_id_token and apple_authorization_code must be sent together")
	}
	if b.AppleRedirectURI == "" {
		return nil
	}
	if b.AppleIDToken == "" {
		return errors.New("apple_redirect_uri needs apple_id_token and apple_authorization_code")
	}
	if !isAbsoluteHTTPSURL(b.AppleRedirectURI) {
		return fmt.Errorf("apple_redirect_uri must be an absolute https URL without a fragment, at most %d characters", maxAppleRedirectURILen)
	}
	return nil
}

func isAbsoluteHTTPSURL(raw string) bool {
	if utf8.RuneCountInString(raw) > maxAppleRedirectURILen || strings.Contains(raw, "#") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
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
