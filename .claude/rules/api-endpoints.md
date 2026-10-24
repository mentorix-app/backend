# API endpoints

Canon: [api/openapi.yaml](../../api/openapi.yaml). Do not duplicate full path lists in docs.

## JSON

| Rule | Example |
| ---- | ------- |
| Keys | `snake_case` | `exercise_id`, `sort_order` |
| Enum values | `snake_case` | `muscle_gain`, `for_time` |
| Exceptions | `token_type: "Bearer"`; single word `email` | |

Shared auth DTOs: [internal/auth/apitypes.go](../../internal/auth/apitypes.go).

## URL paths

| Rule | Example |
| ---- | ------- |
| Segments | kebab-case | `publish-update`, `program-assignment` |
| Resources | plural nouns | `/programs`, `/weeks`, `/blocks` |
| Path params | `{name}_id` in OpenAPI; Echo `:name_id` | `week_id`, `block_id`, `item_id` |
| Root resource | `:id` on mount group | `/programs/:id` |
| No trailing slash | `/programs` not `/programs/` | |

## HTTP verbs

| Intent | Method |
| ------ | ------ |
| Read / list | `GET` |
| Create | `POST` |
| Full replace | `PUT` |
| Partial update | `PATCH` |
| Delete | `DELETE` |
| Reorder (full id list) | `PUT` … `/reorder` |
| Bulk delete (body id list) | `DELETE` on collection, e.g. `DELETE /exercises` |

## Route shape

A collection route sits under its parent and needs the parent context in the path:

```text
POST   /parents/{parent_id}/children
PUT    /parents/{parent_id}/children/reorder
```

An instance route identifies the row by UUID and uses the shortest path that still scopes ownership:

```text
PATCH  /…/blocks/{block_id}
PUT    /…/blocks/{block_id}/exercises/{item_id}
```

Use `week_id` (not only `program_id`) when checking block/day belongs to week.

An action route adds a verb segment to an instance or collection; path param names never contain a verb:

```text
POST …/blocks/merge          POST …/blocks/{block_id}/ungroup
POST …/blocks/{block_id}/move
POST …/blocks/{block_id}/exercises/{item_id}/extract
POST /programs/{id}/publish
```

`sort_order` in the body is optional. Omit it to append at the end (position `1..N` in the parent).

## Program week subtree

Prefix: `/programs/{id}/weeks/{week_id}`

| Scope | Path pattern |
| ----- | ------------ |
| Week | `POST /weeks`, `PUT /weeks/reorder`, `DELETE /weeks/{week_id}` |
| Day collection | `POST /days`, `PUT /days/reorder`, `DELETE /days/{day_id}` |
| Block collection (in day) | `POST /days/{day_id}/blocks`, `PUT …/blocks/reorder`, `POST …/blocks/merge` |
| Block instance | `PATCH /blocks/{block_id}`, `DELETE /blocks/{block_id}`, `POST …/ungroup`, `POST …/move`, `POST …/copy`, `PUT …/blocks/{block_id}/clients` |
| Block exercises | `POST /blocks/{block_id}/exercises`, `PUT …/exercises/reorder`, `PUT\|DELETE …/exercises/{item_id}`, `POST …/extract`, `POST …/move`, `POST …/exercises/{item_id}/duplicate` |

Day id **only** where the operation is about the day's block list (create, reorder, merge). Block `block_id` ops sit at week level.

## Other mounts

| Prefix | Notes |
| ------ | ----- |
| `/auth/*` | auth actions + `/auth/me` |
| `/exercises` | catalog CRUD |
| `/admin/trainers/{user_id}/plan` | grant (`PUT`) or revoke (`DELETE`) a trainer plan |
| `/trainer/clients/program-assignment` | bulk assign (`PUT`); read one client via `GET …/{client_user_id}/program-assignment` |
| `/health`, `/health/ready` | no `created_at` |

`POST /telegram/webhook` (`cmd/api/main.go`) is called only by Telegram. It is deliberately outside `api/openapi.yaml` and the contract test.

## GET entities

Include `created_at` when column exists (except health).

## Errors

Every error body carries `code` and `message`, except the `503` of `GET /health/ready`, which answers with its readiness body. The body is `httpx.ErrorBody`, `{"code": "...", "message": "..."}`, except the quota refusal, a superset (`subscription.QuotaErrorBody`). `httpx.ErrorHandler`, which `app.Mount` installs, writes it.

- `code` is a stable `snake_case` identifier clients branch on; `message` is text for people and may change. OpenAPI `ErrorResponse` mirrors the type.
- Default: the code of the HTTP status (`not_found`, `conflict`, ...). A plain `echo.NewHTTPError` is enough.
- A specific code, `httpx.NewError(status, httpx.CodeX, msg)`, only where one status has several meanings or the app must branch. `NewErrorCause` keeps a cause for the log. List the code on the operation in OpenAPI.
- One text, one code: a sentinel or shared message with a code gets it at every place that turns it into a response. Grep it across `internal/`.
- An error whose message is a struct is written as it is, so the struct must carry `code` and `message` itself (`subscription.QuotaErrorBody` is the only one).
- The cause never reaches the body, also with `e.Debug`. A test that checks codes needs an Echo with `httpx.ErrorHandler`; a bare `echo.New()` renders Echo's default body.

## New endpoint checklist

1. Echo route (match patterns above), mounted from `internal/app/routes.go` (shared by `cmd/api` and the contract test)
2. `api/openapi.yaml`
3. `internal/apicheck/schema.go` if new schemas (every request body schema needs a binding); bind the handler's exported body type, never an anonymous struct copy
4. `docs/features/<feature>.md`: behavior only, not path inventory

Run: `go test ./internal/apicheck/...`
