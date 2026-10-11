# Plan: Apple sign-in return for Android

**Source PRD**: `.claude/prds/mobile-client-sign-in.prd.md`
**Selected Milestone**: 5. Apple sign-in return for Android (was open question 1)
**Complexity**: Small

## Summary

On Android, Sign in with Apple runs in a browser tab. Apple finishes by making the browser POST a form to a return address on our server, and the server has to send the browser back to the app. This plan adds that address: `POST /auth/apple/callback` answers `307` with an `intent://` link for the app's package. It creates no session and checks no token. The app then sends the `id_token` to the existing `POST /auth/apple`, which already accepts the Services ID as audience.

## Sources checked

- Apple, "Incorporating Sign in with Apple into other platforms": with `response_mode=form_post` the result arrives as an HTTP `POST` with an `application/x-www-form-urlencoded` body. Fields: `code` (single use, valid five minutes), `id_token`, `state`, `user` (a JSON string, first authorization only). On failure: `error` and `state`; a cancelled sign-in is `error=user_cancelled_authorize`.
- `sign_in_with_apple` 8.2.0 README (context7 has no entry for the package, so pub.dev was read): the server redirects to `intent://callback?${PARAMETERS FROM CALLBACK BODY}#Intent;package=YOUR.PACKAGE.IDENTIFIER;scheme=signinwithapple;end`, with the url-encoded body as the parameters. Its example server and the community ports for other languages answer with `307`.
- The Flutter developer sent the same link format, Android package `com.mentorix.mobile` and Services ID `com.mentorix.mobile.signin`.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Path | `POST /auth/apple/callback`, public, no JWT | The caller is the user's browser on Apple's behalf; the path fits `/auth/*` |
| Input | Form body only (`PostForm`); the query string is ignored | Apple sends a form; nothing else should reach the app |
| Output | `307`, `Location: intent://callback?<fields>#Intent;package=<package>;scheme=signinwithapple;end` | The format and status the package documents and its users run |
| Fields | Every received form field, re-encoded with `url.Values.Encode()` | The package asks for the whole body. Re-encoding escapes `#`, `;` and `&`, so a caller cannot change the part after `#` |
| Package | New setting `APPLE_ANDROID_PACKAGE`, checked as an Android package name at startup; empty answers `503` | The redirect target must not depend on the request. Same shape as `APPLE_CLIENT_IDS` |
| Size | Route-level body limit of 16 KB, over it `413` | The form holds one ID token and a few short values. The global limit is 1 MB, which would become a 1 MB `Location` header |
| Empty or non-form body | `400` | Nothing to hand to the app |
| Caching and logs | `Cache-Control: no-store`; the handler logs nothing from the body | The link carries the ID token and the authorization code. The request log holds method, path and status only |
| Rate limit | None | No database, Redis or session work happens here |
| Contract | Listed in `api/openapi.yaml` with a form body and a `307` answer | apicheck needs a Go binding only for JSON bodies (`internal/apicheck/openapi.go:167`) |

Out of scope: checking `state` (the app starts the flow, so only the app can compare it), exchanging `code` with Apple, storing anything, account deletion, a second return address for a future production host.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Naming | `internal/auth/handlers.go:71` | One handler method per route, mounted in `Handlers.Mount` |
| Options | `internal/auth/handlers.go:52` | `HandlersOption` (`WithSubscriptions`) for optional settings |
| Errors | `internal/auth/handlers.go:213` | `echo.NewHTTPError(status, message)`; `503` when the feature is not configured |
| Config | `internal/config/config.go:139`, `:165` | Read in `Load`, validate with a small function, fail startup on a bad value |
| Tests | `internal/auth/handlers_test.go:207` | `testAuthHandlers` plus `httptest` requests, table-driven where the cases share a shape |
| Contract | `api/openapi.yaml:145` | The `/auth/apple` entry |

## Files to Change

| File | Action | Why |
|---|---|---|
| `internal/config/config.go`, `internal/config/config_test.go` | UPDATE | `APPLE_ANDROID_PACKAGE` and its validation |
| `internal/auth/handlers.go` | UPDATE | Option, route, `AppleCallback` handler |
| `internal/auth/apple_callback_test.go` | CREATE | Handler tests |
| `internal/app/routes.go` | UPDATE | Pass the setting to the handlers |
| `api/openapi.yaml` | UPDATE | The new path |
| `docs/features/auth.md`, `docs/environments.md`, `.env.example` | UPDATE | Behaviour and the new setting |
| `.claude/prds/mobile-client-sign-in.prd.md` | UPDATE | Milestone row, open question 1 |

## Tasks

### Task 1: Setting

- **Action**: `Config.AppleAndroidPackage` from `APPLE_ANDROID_PACKAGE`. A non-empty value must match `^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`, otherwise `Load` returns an error.
- **Mirror**: `validatePageURL` and its tests.
- **Validate**: tests first for a valid name, an empty value, and bad values (`com.x;S.evil=1`, a value with a space, a single segment); `go test ./internal/config/...`.

### Task 2: Handler

- **Action**: `WithAppleAndroidPackage(pkg)` option, `e.POST("/auth/apple/callback", h.AppleCallback, middleware.BodyLimit("16K"))`, handler as decided above.
- **Mirror**: `Handlers.Apple`.
- **Validate**: tests written first and seen failing:
  - `code`, `id_token`, `state`, `user` give `307`, the exact `Location` and `Cache-Control: no-store`;
  - `error=user_cancelled_authorize` with `state` gives `307` with both fields;
  - values holding `#`, `;`, `&`, `=`, a space, non-ASCII text and a `user` JSON string come out escaped, the `Location` has exactly one `#` and ends with `#Intent;package=com.mentorix.mobile;scheme=signinwithapple;end`;
  - a field in the query string is not copied;
  - an empty form and a JSON body give `400`;
  - an empty setting gives `503`;
  - a body over 16 KB gives `413`;
  - `GET` gives `405`.

### Task 3: Contract and docs

- **Action**: OpenAPI path (`appleSignInCallback`, form body, `307`, `400`, `413`, `503`); one bullet in `docs/features/auth.md`; the setting in `docs/environments.md` and `.env.example`; PRD row and open question.
- **Validate**: `go test ./internal/apicheck/...`, `./scripts/docs-check.sh`.

### Task 4: Proof before Gate 2

- **Action**: full `make check-ci` without agent variables; a local HTTP run with `CORS_ALLOW_ORIGINS` set to another origin and `Origin: https://appleid.apple.com` on the request, to see that the CORS middleware lets the form through.

### Task 5: After the merge

- **Action**: with the owner's word, set `APPLE_ANDROID_PACKAGE=com.mentorix.mobile` on stage, confirm `307` and the `Location` with a form request, then send the Flutter developer the note: the address works, and the app must pass `state` and compare it on return.

## Validation

```bash
go test ./internal/config/... ./internal/auth/... ./internal/apicheck/...
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| The browser on a real Android device does not open the app from the redirect | Low | Format and status are the ones the package documents. The final proof is the developer's run on a device; it cannot be checked from here |
| A forged form makes a victim's app receive someone else's credentials | Low | The server cannot tell a forged form from Apple's, and the form can carry the attacker's own genuine token, which `POST /auth/apple` accepts. Only the app can stop this: a fresh random `state` per flow, compared on return before the token is used, and a callback ignored when no flow is pending. Goes into the note for the developer |
| A wrong package value on stage | Low | The value is checked at startup; a failed start leaves the previous instance serving |
| The ID token ends up in logs | Low | The handler logs nothing from the body and the request log has no body or headers |

## Acceptance

- [ ] All tasks complete
- [ ] Validation passes
- [ ] Patterns mirrored, not reinvented
- [ ] Web cabinet, bot and existing auth endpoints unchanged
