package http

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
)

const (
	MsgInternal            = "internal"
	MsgUnauthorized        = "unauthorized"
	MsgInvalidJSON         = "invalid json"
	MsgInvalidID           = "invalid id"
	MsgInvalidUserID       = "invalid user_id"
	MsgUserNotFound        = "user not found"
	MsgExerciseNotFound    = "exercise not found"
	MsgProgramNotFound     = "program not found"
	MsgForbidden           = "forbidden"
	MsgInvalidToken        = "invalid token"
	MsgMissingAuth         = "missing authorization"
	MsgMissingBearerToken  = "missing bearer token"
	MsgTrainerRoleRequired = "trainer role required"
	MsgAdminRoleRequired   = "admin role required"
)

// Specific error codes. An error without one gets the code of its HTTP status.
// A message text that has a code here has it in every response that carries the
// text, so a client can rely on the pair.
const (
	CodeRateLimited             = "rate_limited"
	CodeEmailTaken              = "email_taken"
	CodeInvalidCredentials      = "invalid_credentials"
	CodeProviderNotConfigured   = "provider_not_configured"
	CodeInvalidIDToken          = "invalid_id_token"
	CodeInvalidLinkCode         = "invalid_link_code"
	CodeTelegramLinkUnavailable = "telegram_link_unavailable"
	CodeTelegramLinkConflict    = "telegram_link_conflict"
	CodeTelegramAlreadyLinked   = "telegram_already_linked"
	CodeMissingRefreshToken     = "missing_refresh_token"
	CodeInvalidRefreshToken     = "invalid_refresh_token"
	CodeUserNotFound            = "user_not_found"
	CodeMissingAuthorization    = "missing_authorization"
	CodeInvalidToken            = "invalid_token"
	CodeClientLimitReached      = "client_limit_reached"
	CodeSelfInvite              = "self_invite"
	CodeInviteNotFound          = "invite_not_found"
	CodeInviteExpired           = "invite_expired"
	CodeInviteConsumed          = "invite_consumed"
	CodeInviteNotConfigured     = "invite_not_configured"
	CodeClientNotLinked         = "client_not_linked"
	CodeAdminCannotAccept       = "admin_cannot_accept"
	CodeClientBlocked           = "client_blocked"
	CodeProgramNotPublished     = "program_not_published"
	CodeClientNotFound          = "client_not_found"
	CodeQuotaExceeded           = "quota_exceeded"

	CodeAccountDeletionUnavailable    = "account_deletion_unavailable"
	CodeAppleAuthorizationRequired    = "apple_authorization_required"
	CodeAppleAccountMismatch          = "apple_account_mismatch"
	CodeInvalidAppleAuthorizationCode = "invalid_apple_authorization_code"
	CodeAppleRevocationFailed         = "apple_revocation_failed"
)

// ErrorBody is the JSON body of every error response. Code is a stable
// identifier clients branch on; Message is text for people and may change.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// codedError is the marker NewError leaves in HTTPError.Internal. It holds the
// specific code and an optional cause. The request log prints it as the internal
// error, so the log line shows the code and the cause while the response body
// shows neither the cause nor the marker.
type codedError struct {
	code  string
	cause error
}

func (e *codedError) Error() string {
	if e.cause == nil {
		return "error_code=" + e.code
	}
	return "error_code=" + e.code + ": " + e.cause.Error()
}

func (e *codedError) Unwrap() error { return e.cause }

// NewError returns the usual *echo.HTTPError with a specific code. The message
// stays a string, so code that reads Message keeps working. Use NewErrorCause to
// attach a cause, because SetInternal replaces the marker that holds the code.
func NewError(status int, code, message string) *echo.HTTPError {
	return NewErrorCause(status, code, message, nil)
}

// NewErrorCause is NewError that also keeps the cause for the log and errors.Is.
// The cause never reaches the response body.
func NewErrorCause(status int, code, message string, cause error) *echo.HTTPError {
	return echo.NewHTTPError(status, message).SetInternal(&codedError{code: code, cause: cause})
}

// ErrorHandler is the echo.HTTPErrorHandler of the API. It follows Echo's default
// handler and renders ErrorBody. Unlike the default it never puts the internal
// error into the body, whatever e.Debug is.
func ErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	he, ok := err.(*echo.HTTPError)
	if !ok {
		he = &echo.HTTPError{Code: http.StatusInternalServerError, Message: http.StatusText(http.StatusInternalServerError)}
	} else if inner, ok := he.Internal.(*echo.HTTPError); ok {
		he = inner
	}

	if c.Request().Method == http.MethodHead {
		err = c.NoContent(he.Code)
	} else {
		err = c.JSON(he.Code, errorPayload(he))
	}
	if err != nil {
		c.Echo().Logger.Error(err)
	}
}

// errorPayload is the value written as JSON. A string or error message becomes
// ErrorBody; any other value, such as a struct with its own fields, is written
// as it is.
func errorPayload(he *echo.HTTPError) any {
	switch m := he.Message.(type) {
	case string:
		return ErrorBody{Code: errorCode(he), Message: m}
	case json.Marshaler:
		return m
	case error:
		return ErrorBody{Code: errorCode(he), Message: m.Error()}
	default:
		return he.Message
	}
}

func errorCode(he *echo.HTTPError) string {
	if coded, ok := he.Internal.(*codedError); ok && coded.code != "" {
		return coded.code
	}
	return codeForStatus(he.Code)
}

// codeForStatus is the code of an error that has no specific one.
func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusGone:
		return "gone"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusUnsupportedMediaType:
		return "unsupported_media_type"
	case http.StatusUnprocessableEntity:
		return "unprocessable_entity"
	case http.StatusTooManyRequests:
		return "too_many_requests"
	case http.StatusInternalServerError:
		return "internal_error"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	}
	if status >= http.StatusInternalServerError {
		return "internal_error"
	}
	return "bad_request"
}
