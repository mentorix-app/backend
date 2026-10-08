# Plan: Mobile client sign-in, Google

**Source PRD**: `.claude/prds/mobile-client-sign-in.prd.md`
**Selected Milestone**: 1. Google sign-in
**Complexity**: Medium

## Summary

Add `POST /auth/google`: the app sends a Google ID token, the backend verifies it and answers with an access token and a refresh token in the JSON body. The first sign-in creates a user with the `client` role. `POST /auth/refresh` and `POST /auth/logout` also accept the refresh token in the JSON body, so the app keeps a session without a browser cookie. The web cabinet path (cookie, no body) does not change. No migration.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Token check | `github.com/coreos/go-oidc/v3`: signature, issuer, expiry; audience checked against `GOOGLE_CLIENT_IDS` | One library serves Apple in milestone 2. Compared with `google.golang.org/api/idtoken` (Google only, large dependency tree) and `golang-jwt` plus a hand-written key fetch (more own code) |
| App session | Refresh token in the JSON body, both ways | The usual shape for native apps. Alternative considered: the app stores the cookie itself; zero backend work but awkward for the Flutter developer |
| Which mode | A request with `refresh_token` in the body gets the new one in the body and no `Set-Cookie`. A request without it works as today | Web behaviour stays byte for byte |
| Account creation | Identity `(google, sub)`. New user: role `client`, name from the token, email stored only when Google marks it verified | No joining by email (PRD) |
| Not configured | Empty `GOOGLE_CLIENT_IDS` answers `503` | Client ids do not exist yet; stage keeps working |
| Provider outage | Any verification failure answers `401` and is logged | The reverted code separated "Google is down" from "bad token" with three extra types. Not worth it here |

Left out on purpose: nonce check, token reuse detection, session families, a separate limiter, account linking.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Naming | `internal/auth/const.go:5` | `Provider*` constants; request types in `internal/auth/apitypes.go` |
| Errors | `internal/auth/handlers.go:151` | Sentinel error in the service, mapped to `echo.NewHTTPError` in the handler |
| Create once | `db/queries/auth.sql` `InsertAuthIdentityIfAbsent` | Insert with `ON CONFLICT DO NOTHING`; on zero rows roll back and read the existing identity |
| Data access | `internal/auth/store.go:30` | One transaction per write path through `s.q.WithTx(tx)` |
| Rate limit | `internal/auth/handlers.go:150` | `h.limiter.AllowLogin` by IP, fails open |
| Tests | `internal/auth/handlers_test.go`, `service_test.go` | Handler tests with a fake service, service tests with a fake store |
| Routes | `internal/auth/handlers.go:68`, `internal/app/routes.go:75` | Routes in `Handlers.Mount`, wiring in `app.Mount` |

## Files to Change

| File | Action | Why |
|---|---|---|
| `go.mod`, `go.sum` | UPDATE | add `github.com/coreos/go-oidc/v3` |
| `internal/auth/idtoken.go` | CREATE | `IDTokenVerifier` interface and the go-oidc implementation with a 5 s key fetch timeout |
| `internal/auth/idtoken_test.go` | CREATE | verifier against a local key server: good token, wrong audience, wrong issuer, expired, bad signature |
| `internal/auth/const.go` | UPDATE | `ProviderGoogle`, `ErrInvalidIDToken`, `ErrProviderNotConfigured` |
| `internal/auth/apitypes.go` | UPDATE | `IDTokenRequest`, `RefreshRequest`; `refresh_token` in `TokenResponse` (omitted for cookie flows) |
| `internal/auth/service.go` | UPDATE | `SignInWithGoogle`; shared session issue helper used by the new path only |
| `internal/auth/store.go` | UPDATE | `FindOrCreateClientByIdentity` in one transaction |
| `internal/auth/handlers.go` | UPDATE | `Google` handler; body mode in `Refresh` and `Logout` |
| `internal/auth/*_test.go` | UPDATE | handler and service cases for the above; cookie flows asserted unchanged |
| `internal/db/storetest/` | CREATE | integration test: two parallel first sign-ins end with one user |
| `internal/config/config.go` | UPDATE | `GOOGLE_CLIENT_IDS` (comma separated) |
| `internal/app/routes.go` | UPDATE | build the verifier, pass it to the handlers |
| `api/openapi.yaml`, `internal/apicheck/schema.go` | UPDATE | new path, request bodies, `refresh_token` |
| `docs/features/auth.md`, `docs/status.md`, `docs/environments.md`, `docs/architecture.md`, `.env.example` | UPDATE | behaviour, env variable, library choice |

## Tasks

### Task 1: ID token verifier
- **Action**: interface plus go-oidc implementation; issuer and key URL are constructor arguments so the test can point at a local server.
- **Mirror**: small interface next to its consumer, as `credentialService` in `handlers.go`.
- **Validate**: `go test ./internal/auth/ -run IDToken`

### Task 2: Find or create the client
- **Action**: store method and service method; one user for concurrent first sign-ins; display name cut to 100 runes.
- **Mirror**: `RegisterTrainerEmailPassword`, `InsertAuthIdentityIfAbsent`.
- **Validate**: `go test ./internal/auth/...` and `go test -tags integration ./internal/db/storetest/...`

### Task 3: `POST /auth/google`
- **Action**: handler, limiter, error mapping (`400` bad body, `401` bad token, `429`, `503` not configured), route, OpenAPI, apicheck binding.
- **Validate**: `go test ./internal/auth/... ./internal/apicheck/...`

### Task 4: Refresh and logout with the token in the body
- **Action**: optional `refresh_token` in the body of both endpoints; body mode never sets or clears a cookie.
- **Validate**: handler tests for both modes; local HTTP run: sign in as a trainer, refresh once with the cookie and once with the body.

### Task 5: Docs and frontend note
- **Action**: docs listed above; `/changelog` text for the Flutter developer.
- **Validate**: `./scripts/docs-check.sh`

## Validation

```bash
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

Plus a local HTTP run of the new and changed endpoints before Gate 2, and the cabinet check on stage after the deploy.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Real Google sign-in cannot be checked until client ids exist | Certain | Verifier tested against a local key server; the real check is recorded as open in the PRD |
| Body mode changes the cabinet's refresh | Low | Mode is chosen only by the presence of `refresh_token` in the body; cookie tests stay as they are |
| Review rounds add mechanisms again | Medium | Findings outside this plan go to the owner as a question, not into the diff |
| A Google outage looks like a bad token to the app | Low | Logged on the server; revisit if it happens |

## Acceptance

- [ ] A valid Google token creates one client account and returns both tokens in the body
- [ ] A second sign-in with the same Google account returns the same user
- [ ] The app can refresh and sign out with the body token; the cabinet does it with the cookie as before
- [ ] `make check-ci` passes; docs and OpenAPI match the code
