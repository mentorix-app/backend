# Plan: Mobile client sign-in, invite in the app

**Source PRD**: `.claude/prds/mobile-client-sign-in.prd.md`
**Selected Milestone**: 4. Invite in the app
**Complexity**: Small

## Summary

Add `POST /client/invites/accept`: a signed-in client sends the trainer's invite and becomes that trainer's client. It runs the same transaction as the bot's accept, with the same plan limit check, and differs only in where the user comes from: the access token, not a Telegram id. No migration.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Which invite | The existing one from `POST /trainer/invites`. The trainer shares the same link as today | No second kind of invite |
| What the app sends | `token`: the bare token, `inv_<token>`, or the whole invite link | The client may paste the link the trainer sent; the backend takes the token out of it |
| Shared code | The body of `Store.AcceptInvite` becomes one private function with a "who is the user" step; the bot path and the app path both call it | One set of rules: lock, expiry, consumed, self invite, limit |
| Who may call | Any signed-in user. The `client` role is granted when missing | A trainer who is also someone's client is allowed by the data model; accepting one's own invite stays refused |
| Answer | `200` with `trainer_id`, `trainer_display_name`, `already_linked` | Same result the bot gets |
| Errors | As in the bot: `404` unknown, `410` expired, `409` used by someone else or no free places (`quota_exceeded`), `422` own invite | Existing mapping in `trainerclient.HTTPErrorFrom` |

Left out on purpose: a notification to the trainer, choosing the "active trainer" for the bot (the bot already asks when it is not set), invites created for a specific person, leaving a trainer.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Transaction | `internal/trainerclient/store.go:272` `AcceptInvite` | Lock the invite, resolve the user, self check, link under the trainer lock, consume |
| Errors | `internal/trainerclient/errors.go` `HTTPErrorFrom` | Sentinel to status |
| Route | `internal/auth/handlers.go` `Mount` | Per-route JWT middleware, so unknown paths under `/client` still answer 404 and the signed-link `/client/analytics` stays public |
| Tests | `internal/trainerclient/handlers_invite_test.go`, storetest invite tests | Fake service for handlers; integration for the transaction |

## Files to Change

| File | Action | Why |
|---|---|---|
| `internal/trainerclient/store.go` | UPDATE | shared private accept; `AcceptInviteAsUser(ctx, token, userID)` |
| `internal/trainerclient/service.go`, `model.go` | UPDATE | service method, token extraction, request and response types |
| `internal/trainerclient/handlers.go` | UPDATE | `POST /client/invites/accept` |
| `internal/trainerclient/*_test.go`, `internal/db/storetest/` | UPDATE | handler, token extraction, integration cases |
| `api/openapi.yaml`, `internal/apicheck/schema.go` | UPDATE | path, body, response |
| `docs/features/trainer-invites.md`, `docs/status.md` | UPDATE | behaviour |
| `.claude/prds/mobile-client-sign-in.prd.md` | UPDATE | milestone statuses |

## Tasks

### Task 1: Accept as a known user
- **Action**: extract the shared accept; add the by-user variant (user must exist; grant `client`). Integration tests: link created and invite consumed; repeat by the same user is `already_linked`; used by someone else; expired; own invite; no free places leaves the invite unused; the bot path still passes its existing tests untouched.
- **Validate**: `go test -tags integration ./internal/db/storetest/...`

### Task 2: Endpoint
- **Action**: handler with JWT, token extraction (bare, `inv_` prefix, full link), status mapping, OpenAPI, apicheck.
- **Validate**: `go test ./internal/trainerclient/... ./internal/apicheck/...`

### Task 3: Docs and PRD

## Validation

```bash
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

Plus a local HTTP run: a local trainer creates an invite, a prepared app account accepts it by token and by full link, then every refusal; after that the Telegram link from milestone 3 must answer `409` for this account, because it now holds data.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| The refactor changes the bot's accept | Low | Existing bot and invite tests stay as they are; the shared function keeps the same order of steps |
| A client accepts an invite before linking Telegram and then cannot merge | Medium | By design (owner's decision); the app asks about Telegram first, using `telegram_linked` |

## Acceptance

- [ ] A client without Telegram becomes a trainer's client from the app
- [ ] The plan limit and every refusal behave as in the bot
- [ ] The bot's invite flow is unchanged
- [ ] `make check-ci` passes; docs and OpenAPI match the code
