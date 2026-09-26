# Program block visibility

**Status:** implemented  
**Code:** `internal/program/` — `block_visibility.go`, `store_block_clients.go`, `handlers_blocks.go`  
**Related:** [program-blocks.md](program-blocks.md), [programs.md](programs.md), [garbage-cleanup.md](garbage-cleanup.md), [telegram-bot.md](telegram-bot.md)

## Purpose

A trainer can limit a day's block to a subset of program clients — the same day can appear differently to different clients (for example, different workload while keeping the same week structure). Visibility rules apply at the block level, not to individual exercises.

## Database

Rules table: `program_block_clients` (migration `000026_program_block_visibility`; block tree overall — [database-naming.md](../../.claude/rules/database-naming.md) § Program trees):

| Column | Type | Note |
| ------ | ---- | ---- |
| `id` | uuid PK | |
| `program_id` | uuid FK → `programs` | `ON DELETE CASCADE` |
| `block_key` | uuid | not FK — stable key, not `program_week_day_blocks.id` |
| `client_user_id` | uuid FK → `users` | `ON DELETE CASCADE` |
| `created_at`, `created_by` | | |

UNIQUE `(program_id, block_key, client_user_id)`. No rows for `(program_id, block_key)` means the block is visible to all program clients; there is no separate boolean flag.

The same migration added `block_key uuid NOT NULL DEFAULT gen_random_uuid()` to both `program_week_day_blocks` and `program_version_week_day_blocks`. This lets the rule survive `publish` and `discard-unpublished`: working copy and versions reuse the same `block_key` instead of getting a new `id` on copy, so the rule continues to point to the same logical block across any tree.

## Rules

- **Empty list = shared block.** `client_user_ids: []` means the block is visible to all program clients; non-empty list means only those clients see it. The field always serializes as `[]`, never as `null` — enforced in code (`store.go`) and OpenAPI (`required`, no `nullable`).
- **Only assigned clients.** Each id in `client_user_ids` must be a client with an active assignment to this program, otherwise `400` (`ErrClientNotAssignedToProgram`).
- **Day invariant.** Every day must have at least one shared block. The check runs only when transitioning from shared to limited (changing client list for an already limited block or removing a limit does not break the invariant); violation → `400` (`ErrLastSharedBlock`). Checked twice: when writing the rule — against working copy **and** against each published version with active assignments (clients are on these versions right now, without publish) **and** against the latest version even if no one is assigned to it yet — next assignment (`SetClientProgramAssignment`/`SyncProgramAssignments`) will land there while working copy has unpublished changes. Then again for working copy at `publish`.
- **Versions do not sync.** The rule stores by `(program_id, block_key)`, not by version, so it applies to any version a client is on — `GetVersionDetailForClient` substitutes rules on read. Changing rules needs no `publish` and no `assignments/sync`.
- **Removing a client can expose a block — intentionally.** Removing a client from the program or moving them to another one deletes their `program_block_clients` rows (`DeleteProgramBlockClientsForClient`). If they were the only one in the block's list, the rules for `(program_id, block_key)` become empty, and empty means "shared" — the block is now visible to other assignees on the version they're already on, with no `publish`, no error, and no log entry. Day invariant and publish validation do not catch this: they guard against losing a shared block, not gaining one. This choice was made intentionally, not overlooked; the frontend should ask the trainer the same question it would for manual cleanup of a block's client list.

## Inheritance in block operations

| Operation | Visibility of result |
| --------- | -------------------- |
| Merge (`.../blocks/merge`) | `400` if blocks being merged have different client lists (including "all shared" ≠ "one limited") — no single correct result |
| Ungroup (`.../blocks/{block_id}/ungroup`) | each resulting `single` inherits group client list |
| Extract exercise (`.../exercises/{item_id}/extract`) | new `single` inherits client list from source block |
| Move exercise (`.../exercises/{item_id}/move`) | rule **not transferred** — exercise gets visibility of target block |
| Move block to another day (`.../blocks/{block_id}/move`) | block client list does not change |

## Rule cleanup

- Client removed from program (`clear`/`reassign`) — their rows for **previous** `program_id` are deleted in the same transaction.
- Orphaned rule — `block_key` that exists neither in working copy nor in any saved version of the program; such rows are deleted by best-effort pass after `assign`/`reassign`/`clear`/`sync` and `cleanup.Run` in `janitor`.

Details and call sites — [garbage-cleanup.md](garbage-cleanup.md) § Block visibility rules.

## API

`PUT /programs/{id}/weeks/{week_id}/blocks/{block_id}/clients` — complete list replacement. Contract, error codes, and `client_user_ids` field on `ProgramDayBlock` — `api/openapi.yaml`. Path naming — [api-endpoints.md](../../.claude/rules/api-endpoints.md) § Program week subtree.

## See also

- [program-blocks.md](program-blocks.md) — day block tree
- [programs.md](programs.md) — publish, versions
- [garbage-cleanup.md](garbage-cleanup.md) — auto-cleanup
- [telegram-bot.md](telegram-bot.md) — block filtering for client in bot
