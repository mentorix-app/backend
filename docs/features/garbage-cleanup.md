# Garbage cleanup

**Status:** implemented  
**Code:** `internal/program/`, `internal/trainerclient/`, `internal/cleanup/`, `cmd/janitor/`

## Purpose

Auto-delete data that stops being needed after operations or by TTL.

## Assignments

- One `program_assignments` row per `(trainer_id, client_user_id)`.
- `assign` / `reassign` — `INSERT` or `UPDATE` the same row.
- `clear` (`program_id: null`) — `DELETE` the row.
- Migration `000019`: purge `cancelled`, unique without `WHERE status = 'active'`.

## Program versions

After `assign`/`reassign`/`clear`/`sync` — best-effort `CleanupProgramVersions` for affected programs (delete versions with no active assignments, except one).

On `DELETE /programs/{id}` — first `DELETE` program assignments, cleanup versions, then soft delete.

## Block visibility rules (`program_block_clients`)

- `clear`/`reassign` (`SetClientProgramAssignment`) — in same transaction `DeleteProgramBlockClientsForClient` deletes client rows for **previous** `program_id` (not new).
- Orphaned rule — `block_key` that exists neither in working copy (`program_week_day_blocks`) nor in any saved version (`program_version_week_day_blocks`) of the program; both conditions required, versions because client may be assigned to a version working copy no longer points to.
- `PurgeOrphanProgramBlockClientsForProgram` (scoped) called from `CleanupProgramVersions` and from best-effort pass after `assign`/`reassign`/`clear`/`sync` — when a `block_key` might lose its last version.
- `PurgeOrphanProgramBlockClients` (global) — same without `program_id`, called from `cleanup.Run`. Data model and rules — [program-block-visibility.md](program-block-visibility.md).

## Invites

On `POST /trainer/invites` — inline purge expired/consumed (7d grace) trainer invites.

## Redis

`mentorix:telegram:active_trainer:{id}` — `Delete` on stale in `resolveActiveTrainerID`.

## Janitor

`go run ./cmd/janitor`: purge stale `trainer_invites`, `auth_refresh_sessions` (30d), orphan `program_block_clients` (global, unscoped).

Requires `DATABASE_URL`. Not scheduled (no cron in `render.yaml`, none in `.github/workflows/`) — safety net, not primary mechanism; block visibility rules are cleaned first by best-effort pass above.

## See also

- [trainer-clients.md](trainer-clients.md)
- [programs.md](programs.md)
- [trainer-invites.md](trainer-invites.md)
