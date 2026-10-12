# Plan: Account deletion in the app

**Source PRD**: `.claude/prds/mobile-client-sign-in.prd.md` (scope note: "Deleting the account from inside the app: a separate plan, needed before App Store review")
**Selected Milestone**: 7. Account deletion in the app (new row)
**Complexity**: Medium, two pull requests

## Summary

The App Store does not accept an app that creates accounts and cannot delete them. This plan adds `DELETE /auth/me`: a client deletes their own account from the app, and every row that belongs to them goes with it. For an account that signs in with Apple the server also revokes the Apple authorization, which Apple asks for. Part 1 ships the endpoint and the data deletion. Part 2 adds the Apple revocation and needs a key from the Apple developer account.

## What Apple requires

Read in Apple's documentation on 2026-10-11.

| Requirement | Source | What it means here |
|---|---|---|
| The user starts deletion inside the app. Deactivating the account is not enough. Data the law does not oblige us to keep is deleted too | "Offering account deletion in your app" | A real delete, started by one API call |
| An app with Sign in with Apple revokes the user's tokens through the REST API | Same page, FAQ | A call to `POST https://appleid.apple.com/auth/revoke` |
| `/auth/revoke` takes a refresh token or an access token. Without one, tokens come from validating an authorization code | "Token revocation" | The app sends a fresh code, the server trades it for a refresh token and revokes that |
| The code is single use and lives five minutes. `redirect_uri` goes into the token request only when the authorization request had one | "Token validation" | The code is obtained right before the delete. Android sends its redirect address, iOS sends none |
| The client secret is an ES256 JWT: header `kid`, claims `iss` (Team ID), `iat`, `exp` (six months at most), `aud` `https://appleid.apple.com`, `sub` (the client id) | "Creating a client secret" | Three new settings: Team ID, Key ID, the `.p8` key |
| Without a token or a code the account is still deleted and the user is told to revoke access by hand | TN3194 | A manual path stays possible for support |
| A confirmation or a new sign-in before deletion is allowed. A manual or delayed process is allowed when the user is told | "Offering account deletion in your app", FAQ | Asking for the Apple sheet again is fine. Refusing trainer accounts in the app is fine while support handles them |

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Endpoint | `DELETE /auth/me`, Bearer token, answers `204` | The verb the API rules give to a delete. `DELETE` with a JSON body already exists (`DELETE /exercises`) |
| Who | Accounts whose roles are exactly `client`. An account with the `trainer` or `admin` role, or with no role at all, gets `409 account_deletion_unavailable` | Deleting a trainer would remove programs, other people's assignments and their workout history, and the database refuses it today. The app serves clients. Support handles the rest until it has its own plan |
| What is deleted | Workout completions with the trainer's comments on them, program assignments, the invites the client used, then the user row. The database cascades sign-in identities, roles, sessions, link codes, trainer links and block visibility rules | Everything that describes the person. Nothing of the trainer's own content is touched. A used invite cannot stay without its user: `trainer_invites_consumed_check` wants `consumed_at` and `consumed_by` both set or both empty, and the cleanup job removes used invites after seven days anyway |
| Kept | Nothing: no soft delete, no grace period, no anonymous copy | Apple asks for deletion. A copy would need its own retention rules |
| Schema | No migration. The deletes are explicit statements in one transaction and the foreign keys stay restrictive | A stray delete of a user keeps failing as it does now. Stage has real users |
| Concurrency | The transaction starts by locking the user row (`FOR UPDATE`). The role rule and the Apple rule are read under that lock, and nowhere else | A trainer who assigns a program in the same instant waits and then fails, and a repeat gets the usual answer for a client that is not linked. A role or an Apple sign-in that arrives in the same instant is seen before anything is deleted |
| Apple proof | Fresh at delete time, nothing stored. The body carries `apple_id_token` and `apple_authorization_code` from a new Sign in with Apple, plus `apple_redirect_uri` on Android | No Apple token at rest, no migration, works for accounts that exist already. Apple documents this path |
| Which Apple account | The token is checked by the existing verifier. Its subject must be one of the account's Apple identities, its audience is the client id for the exchange. The `id_token` in Apple's answer to the exchange must carry the same subject before anything is revoked | Stops a delete that revokes somebody else's authorization and leaves the right one alive. Without the second check a code of another Apple account would pass |
| Order | Checks, then Apple, then the database | A failed revocation deletes nothing and can be retried. If the database step fails after the revocation the account is still there: the person signs in again and repeats. Once Apple confirmed, the delete runs on its own context, so a dropped connection does not cancel it |
| Time | One 10 second budget for both Apple calls | The server's write timeout is 15 seconds; two calls of 10 seconds each would outlast it |
| Body | Optional JSON, read from the body only, `Content-Type: application/json`, at most 16000 bytes | The query string never carries a token or a code |
| Apple account, no proof | `422 apple_authorization_required` | The app shows the Apple sheet and repeats the call |
| Key not set | An account with an Apple sign-in gets `503 provider_not_configured`. Other accounts are deleted as usual | Deletion never skips the revocation silently |
| Client secret | Built per request with `golang-jwt`, five minutes of life | Already a dependency. Compared with `Timothylock/go-signin-with-apple` (a new dependency for about sixty lines) and `x/oauth2` (no revoke call, no JWT secret) |
| Sessions | Refresh sessions go with the user. An access token issued before the delete works until it expires (15 minutes by default) and finds nothing: `GET /auth/me` answers `404 user_not_found` | Same as for any deleted account today |
| Telegram | The Telegram sign-in goes with the account. The bot's `active_trainer` key for that Telegram is removed after the commit, best effort. `workout_pending` (30 minutes) and `avatar_checked` (24 hours) expire by themselves | For the bot the person is unknown again. Accepting an invite later creates a new empty account |
| Coming back | Signing in again with the same Google or Apple account creates a new empty account | Nothing is left to return to |
| Logs | One info line with the user id when an account is deleted. Apple failures are logged with Apple's `error` value. The code, the tokens and the client secret never reach a log or an answer | Support can confirm a deletion without personal data in the log |

### Answers of `DELETE /auth/me`

| Status | `code` | When | What the app does |
|---|---|---|---|
| `204` | | Deleted | Clears its tokens |
| `400` | `bad_request` | Invalid JSON or a body that is not `application/json`, one Apple field without the other, `apple_redirect_uri` not an `https` address | Developer error |
| `413` | `payload_too_large` | A body over 16000 bytes | Developer error |
| `400` | `invalid_apple_authorization_code` | Apple refused the code (`invalid_grant`: expired, used, issued for another client) | New Apple sheet, repeat |
| `401` | `invalid_id_token` | `apple_id_token` failed verification | New Apple sheet, repeat |
| `401` | `missing_authorization`, `invalid_token` | No access token or a bad one | Refresh, repeat |
| `403` | `apple_account_mismatch` | The Apple account in the token, or the one the code belongs to, is not the one this account signs in with | Asks for the right Apple account |
| `404` | `user_not_found` | The account is already gone | Treats it as done |
| `409` | `account_deletion_unavailable` | Trainer or admin role, or the account still owns content | Points to support |
| `422` | `apple_authorization_required` | The account has an Apple sign-in and the body has no Apple fields | Apple sheet, repeat |
| `502` | `apple_revocation_failed` | Apple did not answer or answered anything else. `invalid_client` lands here: a wrong `apple_redirect_uri` or a wrong key | "Try later"; we read the log |
| `503` | `provider_not_configured` | The account has an Apple sign-in and the server has no Apple key | "Try later" |

Part 1 answers `204`, `401`, `404`, `409` and, for every account with an Apple sign-in, `503`. Part 2 adds the body and the Apple rows.

Out of scope: storing Apple refresh tokens at sign-in, Apple's server-to-server notifications, deleting trainer or admin accounts, deletion from the bot or the web cabinet, a grace period or undo, a message to the trainer, revoking Google tokens on the server, a new sign-in before deleting a Google account, a rate limit on this endpoint, a second Apple identity on one account (only the proven one is revoked).

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Transaction that deletes a user | `internal/auth/store.go:177` | `LinkTelegram`: `pool.Begin`, `q.WithTx`, `DeleteUserByID`, a foreign key violation mapped to a sentinel through `isReferenceOrCheckViolation` (`store.go:27`) |
| Handler behind the token | `internal/auth/handlers.go:382` | `LogoutAll`: user id from the context, `204` |
| Error mapping with codes | `internal/auth/handlers.go:283` | `TelegramLink`: sentinels to `httpx.NewError` and `httpx.NewErrorCause`, `pgx.ErrNoRows` to `404 user_not_found` |
| Optional provider | `internal/auth/service.go:57`, `internal/app/routes.go:84` | `WithAppleVerifier`: the option is passed only when the settings exist; without it the service answers `ErrProviderNotConfigured` |
| ID token check | `internal/auth/idtoken.go:82` | `oidcVerifier.Verify`: issuer, signature, expiry, audience allow-list |
| Setting checked at start | `internal/config/config.go:175` | `APPLE_ANDROID_PACKAGE`: empty means off, a bad value stops the start |
| `DELETE` with a JSON body | `api/openapi.yaml:620` | `deleteExercises` |
| Body type and its binding | `internal/auth/apitypes.go:33`, `internal/apicheck/schema.go:34` | Exported request type bound in the contract test |
| Tests | `internal/auth/handlers_telegram_link_test.go`, `internal/db/storetest/auth_telegram_link_integration_test.go`, `internal/app/routes_test.go:140` | Handler tests with a fake service, integration tests against `mentorix_test`, a wiring test for `app.Mount` |

No code in the repository calls an outside HTTP API with form bodies and a signed client secret. The Apple client is new; its tests use `httptest`.

## Files to Change

### Part 1: endpoint and data deletion

| File | Action | Why |
|---|---|---|
| `db/queries/auth.sql`, `db/queries/client_workout_completion.sql`, `db/queries/program_assignment.sql`, `db/queries/trainer_invite.sql` | UPDATE | Lock the user row, list the account's identities, delete completions, assignments and used invites of one client. Each delete sits in the query file of its table |
| `internal/db/sqlc/` | REGENERATE | `sqlc generate` |
| `internal/auth/account_deletion.go` | CREATE | Store transaction and service method |
| `internal/auth/handlers.go`, `const.go` | UPDATE | Handler, route, sentinels |
| `internal/http/errors.go` | UPDATE | `account_deletion_unavailable` |
| `internal/app/routes.go` | UPDATE | Pass the `active_trainer` cleanup to auth |
| `internal/auth/*_test.go`, `internal/db/storetest/*`, `internal/app/routes_test.go` | CREATE / UPDATE | Handler, service, integration and wiring tests |
| `api/openapi.yaml` | UPDATE | `deleteMe` under `/auth/me` |
| `docs/features/account-deletion.md`, `docs/README.md`, `docs/status.md`, `docs/features/auth.md`, `docs/features/garbage-cleanup.md` | CREATE / UPDATE | Feature doc, its link and status row, a pointer from auth, the Redis line |
| `.claude/prds/mobile-client-sign-in.prd.md` | UPDATE | Milestone 7 row; milestone 6 to `complete` (it is live) |

### Part 2: Apple revocation

| File | Action | Why |
|---|---|---|
| `internal/config/config.go` and its tests | UPDATE | `APPLE_TEAM_ID`, `APPLE_KEY_ID`, `APPLE_PRIVATE_KEY`: all three or none, the key must parse as a P-256 private key, a literal `\n` in the value is read as a line break |
| `internal/auth/apple_revoke.go` and its test | CREATE | Client secret, code exchange, revocation, ten second timeout |
| `internal/auth/idtoken.go` | UPDATE | The verified claims carry the audience that matched |
| `internal/auth/apitypes.go`, `account_deletion.go`, `handlers.go`, `const.go`, `internal/http/errors.go` | UPDATE | Body type `DeleteAccountRequest`, the Apple step, four codes |
| `internal/app/routes.go` | UPDATE | Build the revoker when the three settings exist. The wiring is tested through the mounted route in `internal/db/storetest`, because nothing observable happens before the first database read |
| `api/openapi.yaml`, `internal/apicheck/schema.go` | UPDATE | Request body, new answers, binding |
| `docs/features/account-deletion.md`, `docs/environments.md`, `.env.example` | UPDATE | Apple step, the three settings (Dashboard only, like the client ids) |

## Tasks

### Part 1

#### Task 1: Deletion transaction

- **Action**: one store method that locks the user row, refuses roles other than exactly `client`, refuses an Apple identity, deletes the client's assignments, completions and used invites, deletes the user and returns the account's Telegram id when it had one. A missing user gives `pgx.ErrNoRows`. A foreign key violation gives the "unavailable" sentinel.
- **Mirror**: `LinkTelegram`.
- **Validate**: integration tests written first. A client with a trainer link, an assignment, completions, a comment, a visibility rule, a session and a used invite is deleted and no row with their id is left. A second client of the same trainer and the trainer's programs, versions and exercises are untouched (row counts before and after). The client's used invite is gone and the trainer's other invites stay. A trainer, an admin and an account with both roles are refused and nothing changes. An unknown id gives `pgx.ErrNoRows`. A second call gives `pgx.ErrNoRows`.

#### Task 2: Service, handler, route

- **Action**: the store refuses an account with an Apple identity under the lock and the service turns that into `ErrProviderNotConfigured`. After the store call the service removes the `active_trainer` key through an injected function, on a context that survives a dropped connection, and writes the log line. Handler `DeleteMe`, route `DELETE /auth/me` behind the token.
- **Mirror**: `LogoutAll`, `TelegramLink`.
- **Validate**: handler tests for `204`, `401`, `404`, `409`, `503` with their codes, on an Echo with `httpx.ErrorHandler`. Service tests: the store's Apple refusal becomes `ErrProviderNotConfigured`; a failed Redis cleanup does not fail the request; a store error is not swallowed. Wiring test in `internal/app`.

#### Task 3: Contract and docs

- **Action**: `deleteMe` in OpenAPI with its codes, the feature doc, status row, PRD rows.
- **Validate**: `go test ./internal/apicheck/...`, `./scripts/docs-check.sh`.

### Part 2

#### Task 4: Settings

- **Action**: read and check the three settings at start. The error text never contains the key.
- **Mirror**: `validateAndroidPackage`.
- **Validate**: table tests: none set, all set, one missing, a key that is not PEM, a key of the wrong type, a value with literal `\n`.

#### Task 5: Apple client

- **Action**: `Revoke(ctx, clientID, subject, code, redirectURI)`: sign the client secret, `POST /auth/token` with `grant_type=authorization_code` (and `redirect_uri` only when given), check that the `id_token` of the answer has the expected subject, Apple's issuer and our client id, then `POST /auth/revoke` with the refresh token and `token_type_hint=refresh_token`. `invalid_grant` becomes the "invalid code" sentinel, another subject the "mismatch" sentinel, everything else the "revocation failed" sentinel with Apple's `error` value as the cause.
- **Mirror**: none in the repository; `oidcVerifier` for the timeout and the constructor shape.
- **Validate**: tests against an `httptest` server with a generated P-256 key. The secret verifies with the public key and carries `kid`, `iss`, `sub`, `aud`, `iat`, `exp`. Form fields are exact; `redirect_uri` is absent when empty. `invalid_grant`, `invalid_client`, a `500`, a timeout and a token answer without `refresh_token` each map to the right sentinel. No error text contains the code, a token or the secret.

#### Task 6: The Apple step in the deletion

- **Action**: body `DeleteAccountRequest`. For an account with an Apple identity: no fields gives `apple_authorization_required`; the token is verified; its subject must match; the revoker runs; then the store. The store takes the Apple subject that was revoked and compares it with the identities it reads under the lock, so an Apple sign-in that arrived in between is still refused. The service reads the identities only to decide whether to ask Apple; the refusal itself stays in the transaction. The Apple fields are read from the JSON body only.
- **Validate**: service tests for every row of the answers table, including "revoker fails, store not called" and "account without Apple ignores the fields". Handler tests for the codes. Wiring test: no settings gives `503` for an Apple account.

#### Task 7: Contract and docs

- **Action**: request body and answers in OpenAPI, binding in apicheck, the Apple section of the feature doc, settings in `docs/environments.md` and `.env.example`.
- **Validate**: `go test ./internal/apicheck/...`, `./scripts/docs-check.sh`.

## Validation

```bash
go test ./internal/auth/... ./internal/config/... ./internal/app/... ./internal/apicheck/...
go test -tags integration ./internal/db/storetest/...
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

Before each Gate 2 a local HTTP run against the local database: an app client with a trainer, an assignment and a completion calls `DELETE /auth/me`; the rows are counted before and after; a trainer token gets `409`; the second call gets `404`.

After each deploy: stage `/health` shows the commit, readiness is green, `DELETE /auth/me` without a token answers `401`, the cabinet is checked under the owner's session. No account on stage is deleted by us.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| The delete removes rows of another person on stage | Low | Every statement is keyed by the user id from the token. Integration tests compare a neighbour client and the trainer's content before and after. Trainers and admins are refused. No schema change |
| The Apple step cannot be proven before the key exists | Certain | Tests against a fake Apple server check both requests and the signed secret. The real proof is the developer deleting a test account on stage: the app leaves the "Sign in with Apple" list of the Apple account and the next sign-in asks for the name and email again. Reported as unverified until then |
| The `.p8` key | Medium | Created by whoever owns Apple team `TSQ6DL62KG`, under Keys with Sign in with Apple enabled for the primary App ID. It goes into the Render Dashboard by hand and never into the chat or the repository. Part 2 can be merged before it exists |
| A wrong `apple_redirect_uri` from the Android app | Medium | Apple answers `invalid_client`, the API answers `502` and the log names Apple's error. Written into the note for the developer |
| Apple is down | Low | Nothing is deleted, the app repeats later |
| Revoked at Apple, then the database step fails | Low | The account still exists; sign in again and repeat |
| The trainer loses a client's history without a message | Certain | Owner's decision at Gate 1 |
| Review rounds add mechanisms | Medium | The out-of-scope list goes into every agent brief; reviewers put such ideas under "Outside the plan" |

## Acceptance

- [ ] A client deletes their account with one call and no row with their id remains
- [ ] Other clients, trainers and their content are unchanged
- [ ] Trainer and admin accounts are refused with a clear code
- [ ] An Apple account is deleted only after Apple accepted the revocation
- [ ] No Apple token is stored and no secret reaches a log or an answer
- [ ] OpenAPI, apicheck, feature doc, status and settings docs match the code
- [ ] `make check-ci` passes for each part
