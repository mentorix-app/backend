# Plan: Mobile client sign-in, Apple

**Source PRD**: `.claude/prds/mobile-client-sign-in.prd.md`
**Selected Milestone**: 2. Apple sign-in
**Complexity**: Small

## Summary

Add `POST /auth/apple` on the mechanism built for Google: the app sends an Apple ID token, the backend verifies it and answers with an access token and a refresh token in the body. The first sign-in creates a `client` user with the identity `(apple, sub)`. No migration.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Token check | The existing go-oidc verifier with issuer `https://appleid.apple.com` and keys `https://appleid.apple.com/auth/keys`; audience from `APPLE_CLIENT_IDS` | Same code path as Google |
| Audiences | `APPLE_CLIENT_IDS` holds the iOS bundle id and, for Android, the Services ID | Tokens from the Android web form carry the Services ID as audience |
| Name | Optional `name` in the request body, used only when the account is created | Apple never puts the name in the token; the app receives it once, on the first authorization |
| `email_verified` | Accept a JSON boolean or the strings `"true"` and `"false"` | Apple sends either form; a strict boolean would answer 401 to valid tokens |
| Shared code | `SignInWithGoogle` and the new method call one private helper | Two providers, one path |
| Not configured | Empty `APPLE_CLIENT_IDS` answers `503` | As with Google |
| Android return page | Not in this PR | On Android Apple posts the result to a web address, which must send the browser back to the app. Its exact shape depends on the Flutter package and the Android package name; the Flutter developer has not started. A small follow-up once that is known |

Left out on purpose: exchanging the authorization code with Apple (needs a signing key and is only required to revoke access when an account is deleted; it belongs to the account deletion plan), nonce, private relay email handling, linking.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Verifier | `internal/auth/idtoken.go` `NewGoogleIDTokenVerifier` | One exported constructor per provider over `newOIDCVerifier` |
| Handler | `internal/auth/handlers.go` `Google` | Limiter, bind, error mapping with `SetInternal`, body answer |
| Service | `internal/auth/service.go` `SignInWithGoogle`, `WithGoogleVerifier` | Option per provider, `ErrProviderNotConfigured` when absent |
| Tests | `internal/auth/idtoken_test.go`, `handlers_google_test.go`, `service_google_test.go` | Local key server; fake service and store |

## Files to Change

| File | Action | Why |
|---|---|---|
| `internal/auth/idtoken.go`, `idtoken_test.go` | UPDATE | Apple constructor; `email_verified` as boolean or string |
| `internal/auth/const.go` | UPDATE | `ProviderApple` |
| `internal/auth/apitypes.go` | UPDATE | `AppleSignInRequest{id_token, name}` |
| `internal/auth/service.go` | UPDATE | `SignInWithApple`, shared private helper, `WithAppleVerifier` |
| `internal/auth/handlers.go` | UPDATE | `Apple` handler and route |
| `internal/auth/*_test.go` | UPDATE | Cases for the above |
| `internal/config/config.go`, `internal/app/routes.go` | UPDATE | `APPLE_CLIENT_IDS`, wiring |
| `api/openapi.yaml`, `internal/apicheck/schema.go` | UPDATE | New path and body |
| `docs/features/auth.md`, `docs/status.md`, `docs/environments.md`, `.env.example` | UPDATE | Behaviour and env variable |
| `.claude/prds/mobile-client-sign-in.prd.md` | UPDATE | Milestone 1 complete, milestone 2 in progress, Android return page as an open question |

## Tasks

### Task 1: Verifier for Apple
- **Action**: constructor; tolerant `email_verified`. Tests: string and boolean forms, Apple issuer.
- **Validate**: `go test ./internal/auth/ -run IDToken`

### Task 2: Service and handler
- **Action**: shared helper, `SignInWithApple` with the optional name (trimmed, cut to 100 runes, ignored for an existing account), `POST /auth/apple` with `400`, `401`, `429`, `503`.
- **Validate**: `go test ./internal/auth/... ./internal/apicheck/...`; the Google tests pass unchanged.

### Task 3: Docs and PRD
- **Validate**: `./scripts/docs-check.sh`

## Validation

```bash
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

Plus a local HTTP run (503 unconfigured, 400, 401 with real Apple keys fetched) and the cabinet check on stage after the deploy.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Real Apple sign-in cannot be checked without an Apple developer account and ids | Certain | Tests against a local key server; real check recorded as open |
| Android cannot finish Apple sign-in until the return page exists | Certain | Named as a follow-up; iOS is not affected |
| Refactoring the Google path breaks it | Low | Google tests stay as they are |

## Acceptance

- [ ] A valid Apple token creates one client account and returns both tokens in the body
- [ ] The name from the request is stored on creation and ignored afterwards
- [ ] Google sign-in and the cookie flows behave as before
- [ ] `make check-ci` passes; docs and OpenAPI match the code
