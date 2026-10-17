# Plan: Fill an empty display name at sign-in

**Source**: the Flutter developer's report of 2026-10-13 (Apple sends the name only on the first authorization; an existing account with an empty name keeps it empty)
**Complexity**: Small

## Summary

`POST /auth/google` and `POST /auth/apple` apply the name only when they create the account. If the account already exists with an empty `display_name`, the name of the sign-in is dropped. After this change a sign-in fills an empty name; a name that is set is never overwritten.

## Decisions

| Topic | Decision | Why |
|---|---|---|
| Which name | The one the sign-in already computes: the token's `name` claim, else the `name` field of the request (Apple puts no name in the token), cleaned and cut to 100 characters | Same rules as on creation |
| When | Only when the stored name is empty after trimming | A name the person chose stays |
| Providers | Google and Apple | One rule for both sign-ins |
| Concurrency | One `UPDATE ... WHERE id = $1 AND btrim(display_name) = ''` | Two sign-ins at once cannot overwrite each other's name |
| Contract | No change in request or response shape; the answer of `GET /auth/me` simply has the name | |

Out of scope: overwriting a non-empty name, the Telegram link path, `PATCH /auth/me`, a migration.

Accepted consequence: `PATCH /auth/me` accepts an empty name, so a person who clears their name gets it back at the next Google sign-in (Google always sends the name) or the next Apple sign-in that carries one.

## Patterns to Mirror

| Category | Source | Pattern |
|---|---|---|
| Fill if empty | `db/queries/auth.sql:103` | `FillUserPrimaryEmailIfEmpty` |
| Sign-in flow | `internal/auth/service.go` `signInWithProvider` | name from the claims, fallback name, `truncateRunes` |
| Store | `internal/auth/store.go` `UpdateUserDisplayName` | small store method over one sqlc query |
| Tests | `internal/auth/service_google_test.go`, `internal/db/storetest/auth_client_identity_integration_test.go` | service tests with a fake store, integration tests on `mentorix_test` |

## Files to Change

| File | Action | Why |
|---|---|---|
| `db/queries/auth.sql`, `internal/db/sqlc/` | UPDATE, REGENERATE | `FillUserDisplayNameIfEmpty` |
| `internal/auth/store.go`, `service.go` | UPDATE | Store method; call it for an existing account when the name is not empty |
| `internal/auth/*_test.go`, `internal/db/storetest/*` | UPDATE | Tests |
| `docs/features/auth.md` | UPDATE | The name rule |

## Tasks

### Task 1: Query and store
- **Action**: `FillUserDisplayNameIfEmpty(user_id, display_name)`, `:execrows`, updates only an empty name.
- **Validate**: integration tests: an empty name is filled; a set name is kept; a blank name of spaces counts as empty; an empty argument changes nothing.

### Task 2: Sign-in
- **Action**: after `FindOrCreateClientByIdentity` returns an existing account and the computed name is not empty, call the store method before loading the profile. A failure of the fill fails the sign-in like any other store error.
- **Validate**: service tests for Google and Apple: existing account with an empty name gets the token name or the request name; existing account with a name keeps it; a new account is unchanged; an empty computed name does not call the store.

### Task 3: Docs
- **Action**: `docs/features/auth.md`: the name is taken on creation and also fills an empty name of an existing account.
- **Validate**: `./scripts/docs-check.sh`.

## Validation

```bash
go test ./internal/auth/... -count=1
go test -tags integration ./internal/db/storetest/... -count=1
env -u AI_AGENT -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT make check-ci
```

Local HTTP run before Gate 2 with a fake account; after the deploy, the developer's next Apple sign-in shows the name in `GET /auth/me` without the client's `PATCH`.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| A cleared name comes back | Low | Accepted consequence, written in the doc |
| A wrong name on a shared device | Low | Only an empty name is filled, by the account's own provider identity |
