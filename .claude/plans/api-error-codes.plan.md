# Plan: stable error codes in API responses

**Source**: request from the mobile developer, approved by the owner on 2026-10-11 ("for the whole API")
**Complexity**: Medium

## Summary

Every error response gets a stable `code` next to `message`. Clients branch on `code`; `message` stays as text for people and may change. One error handler renders the body for the whole API, so the roughly 290 places that build errors do not change unless they need a code of their own.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Body | `{ "code": "...", "message": "..." }`. Message texts stay exactly as they are | Additive for the web cabinet |
| Where it is rendered | One `HTTPErrorHandler` in `internal/http`, installed by `app.Mount` before the routes | One place; covers router errors (404, 405), body limit, panics. `app.Mount` is shared by `cmd/api`, the contract test and the HTTP integration tests, so they all run the handler production runs |
| Default code | From the HTTP status: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `method_not_allowed`, `conflict`, `gone`, `payload_too_large`, `unsupported_media_type`, `unprocessable_entity`, `too_many_requests`, `internal_error`, `service_unavailable` | Every response has a code from day one. Ten of these can be returned today; `gone`, `unsupported_media_type` and `too_many_requests` cover statuses that currently always carry a specific code or never occur |
| Specific codes | Set explicitly where one status has several meanings or the client must branch: sign-in, refresh, Telegram link, invites, quota (list below) | These are the endpoints the app uses |
| How a specific code is set | `httpx.NewError(status, code, message)` returns the usual `*echo.HTTPError`. `Message` stays a string; the code travels as a small marker error in `Internal`, and `httpx.NewErrorCause(status, code, message, cause)` keeps a cause next to it. The handler reads the marker only when it sits directly in `Internal`, so a code buried deeper in an error chain is ignored | No registry, no reflection. Existing code and tests that read `Message` as a string keep working, and the request log line gains the code |
| Debug mode | The handler never adds the cause to the body, whatever `e.Debug` is | Echo's default prints `err.Error()` into the body in debug mode; a public API must not |
| Quota error | Keeps `error`, `resource`, `plan`, `limit`, `usage`; gains `code: "quota_exceeded"` and `message` | The cabinet reads `error` and ignores extra fields (checked in `parseQuotaError.ts`) |
| Naming | snake_case, as other enum values in the API | `.claude/rules/api-endpoints.md` |

Specific codes in this change:

- Auth: `invalid_credentials`, `email_taken`, `rate_limited`, `missing_refresh_token`, `invalid_refresh_token`, `invalid_id_token`, `provider_not_configured`, `missing_authorization`, `invalid_token`, `user_not_found`. Invalid JSON keeps the status-based `bad_request`: the text `invalid json` is produced in 28 places across six packages, and one text must not map to two codes
- Telegram link: `invalid_link_code`, `telegram_link_conflict`, `telegram_already_linked`, `telegram_link_unavailable`
- Invites and trainer-client errors mapped in `trainerclient.HTTPErrorFrom`: `invite_not_found`, `invite_expired`, `invite_consumed`, `client_limit_reached`, `self_invite`, `client_blocked`, `admin_cannot_accept`, `invite_not_configured`, `client_not_linked`, `client_not_found`, `program_not_published`. The sentinels only the Telegram bot produces (`ErrTrainerNotLinked`, `ErrActiveTrainerNotSet`, `ErrTelegramUserNotFound`) keep the status-based code: no HTTP route can return them
- Rule for all of the above: a sentinel error or shared message that gets a specific code gets it at every place that turns it into a response, so one text never has two codes
- Quota: `quota_exceeded`
- Endpoints added after this plan was first written: `GET /client/trainers` needs nothing of its own (the JWT middleware codes and `internal_error` cover it). `POST /auth/apple/callback` keeps the status-based codes, because the browser calls it and the app never reads its error body.

Left out on purpose: specific codes for program, exercise, analytics and comment handlers (about 200 call sites). They get the status-based code now, except where they answer with a text that carries a specific code elsewhere (`client_not_linked`, `client_blocked`, `program_not_published`, `client_not_found`, `user_not_found`): the rule above gives those the same code. A further specific code is added when a client needs to tell two cases apart. No change to statuses or message texts. No localisation of messages.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Shared HTTP helpers | `internal/http/errors.go`, `middleware.go` | Constants and small functions in `internal/http` |
| Structured error body | `internal/subscription/handlers.go` `QuotaErrorBody` | A struct as the error message is rendered as JSON |
| Error mapping | `internal/trainerclient/errors.go` `HTTPErrorFrom` | One switch from sentinel to response |
| Server setup | `internal/app/routes.go` `Mount` | The one place both the server and the tests go through |
| Contract | `internal/apicheck` | OpenAPI schemas bound to exported Go types |

## Files to Change

| File | Action | Why |
|---|---|---|
| `internal/http/errors.go`, tests | UPDATE | `ErrorBody`, `NewError`, status codes table, `ErrorHandler` |
| `internal/app/routes.go` | UPDATE | install the handler in `Mount` |
| `internal/auth/handlers.go`, `middleware.go` | UPDATE | specific codes |
| `internal/trainerclient/errors.go` | UPDATE | specific codes in `HTTPErrorFrom` |
| `internal/program/handlers.go`, `internal/analytics/handlers.go` | UPDATE | the same codes for the same texts (`mapProgramError`, `analytics.HTTPErrorFrom`) |
| `internal/subscription/handlers.go` | UPDATE | `code` and `message` in the quota body |
| tests in those packages, storetest HTTP test | UPDATE | assert `code`; bodies now have two keys |
| `api/openapi.yaml`, `internal/apicheck/schema.go` | UPDATE | `ErrorResponse` schema, shared responses, code lists on the app's endpoints, a note that every error has this body |
| `.claude/rules/api-endpoints.md` | UPDATE | "Errors" section: body, naming, when to add a specific code |
| `docs/features/auth.md`, `trainer-invites.md`, `subscriptions.md`, `trainer-clients.md`, `programs.md`, `trainer-analytics.md`, `client-analytics.md`, `docs/status.md` | UPDATE | codes next to statuses |

## Tasks

### Task 1: Handler and helpers
- **Action**: handler that mirrors Echo's default (committed response, HEAD without body, non-HTTP errors as 500) and adds the code. Tests: string message, explicit code, quota body, router 404 and 405, panic through Recover, HEAD.
- **Validate**: `go test ./internal/http/...`

### Task 2: Specific codes
- **Action**: replace the listed errors with `NewError`; texts unchanged. Tests assert code and message.
- **Validate**: `go test ./internal/auth/... ./internal/trainerclient/... ./internal/subscription/...`

### Task 3: Contract and docs
- **Action**: OpenAPI, apicheck, rule, feature docs.
- **Validate**: `go test ./internal/apicheck/...`, `./scripts/docs-check.sh`

## Validation

```bash
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

Plus a local HTTP run of every listed code, the cabinet check on stage after the deploy (sign in, lists, a quota refusal if one can be shown without writing), and changelog notes for both frontends.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| The cabinet breaks on the new field | Low | Additive only; its quota parser reads named fields; cabinet check after deploy |
| The custom handler differs from Echo's default in an edge case | Medium | Tests for HEAD, committed responses, non-HTTP errors, panics |
| Request log lines change shape | Low | Keep `code=..., message=...` readable; check one line of each kind locally |

## Acceptance

- [ ] Every error response has `code` and `message`, except the `503` of `GET /health/ready`, which keeps its readiness body; message texts are unchanged
- [ ] The two 409 cases of invite accept and of Telegram link have different codes
- [ ] The cabinet works unchanged
- [ ] `make check-ci` passes; OpenAPI and docs list the codes
