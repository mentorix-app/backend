# Program blocks

**Status:** implemented  
**Code:** `internal/program/` (extended)  
**Related:** [programs.md](programs.md)

## Purpose

A day in a program contains an ordered list of **blocks**. A single is one exercise; a group is a block with `block_type` ≠ `single`, created only through merge.

## Database

Tree structure (see [database-naming.md](../../.claude/rules/database-naming.md)):

```text
program_week_days → program_week_day_blocks → program_week_day_block_exercises
program_version_week_days → program_version_week_day_blocks → program_version_week_day_block_exercises
```

### `program_week_day_blocks`

| Column | Type | Note |
| ------ | ---- | ---- |
| `id` | uuid PK | |
| `program_week_day_id` | uuid FK → `program_week_days` | |
| `block_type` | text + CHECK | not `type` |
| `instruction` | text DEFAULT `''` | |
| `sort_order` | int | position in day |
| `created_at` | timestamptz | |
| `modified_at` | timestamptz | |
| `modified_by` | uuid FK → `users` | nullable |
| `block_key` | uuid NOT NULL DEFAULT `gen_random_uuid()` | stable block identity across publish and discard |

`block_type`: `single`, `emom`, `amrap`, `for_time`, `intervals`, `chipper`, `ladder`, `death_by`, `superset`, `complex`, `skill_work`, `strength`, `conditioning`, `gymnastics`, `weightlifting`.

### `program_week_day_block_exercises`

- FK: `program_week_day_block_id` → `program_week_day_blocks`.
- Fields: `exercise_id`, `sort_order`, `sets`, `reps`, `instruction`, `created_at`.

Mirror for version tree (see naming rule).

### `program_block_clients` — block visibility

Who sees the block; table schema, `block_key`, day invariant, and inheritance during merge/ungroup/extract — [program-block-visibility.md](program-block-visibility.md).

**`sort_order`:** after any operation touching order (create, move, reorder, delete, merge, ungroup, extract), the store normalizes siblings to unique `1..N` without gaps. Response from `GET /programs/{id}` returns the tree already sorted by `sort_order`.

### Data migration

Each existing exercise in a day becomes a `single` block with row transfer. Publish copy includes blocks.

## Behavior

| Action | Result |
| ------ | ------ |
| Add exercise to day (catalog, multiselect) | N `single` blocks (`POST .../blocks`) |
| Add exercise to group | `POST .../blocks/{block_id}/exercises` (group exists after merge) |
| Create group | only via `POST .../blocks/merge` (2+ blocks); default `complex`; instruction concatenates instruction from participating **groups** (`\n\n`, empty ones skipped); singles only → `""`; **400** (`ErrValidation`) if participants have different client lists (including "all shared" ≠ "one limited") — no single correct result, merge rejected |
| Ungroup | each exercise becomes `single`, inherits group client list |
| Delete exercise from `single` | block is deleted |
| Delete from group | group remains (0/1/2+); does **not** become `single` |
| Move to another day | only whole `single` or whole group (⋮ → day) |
| Drag-and-drop within day | blocks; exercises in group; extract / to group / between groups |

To move exercise in a group to another day: first extract to `single` (inherits group client list), then move.

## Publish

- `single`: exactly 1 exercise; `sets`/`reps` optional (`null`/omit); when present, string format: digits only (`3`), single `/` (`5/4`), or single `-` (`3-6`).
- Group (`block_type` ≠ `single`): ≥ 1 exercise; empty group is an error; each exercise follows the same `sets`/`reps` rules.
- Group `instruction` may be empty (`""`).

Other validation is in [programs.md](programs.md).

## Implementation plan

### Phase 1 — schema

1. `db/migrations/000014_program_day_blocks.up.sql` — block tables, alter exercises, version tree, data migration, drop `weight_kg`.
2. `db/queries/program.sql`, `program_version.sql` — block CRUD.
3. `sqlc generate`, `make schema-sync`.
4. Update `database-naming.md` (tree), `architecture.md`, `status.md` (migration number).

### Phase 2 — domain and reads

5. Go: `Block`, `BlockType`, `Day.blocks[]`; remove `weight_kg` from DTO.
6. Store: load day with blocks; snapshot in version publish.
7. Store + integration tests.

### Phase 3 — add (UI compatibility)

8. `POST .../days/{day_id}/exercises` — array → N `single` blocks (as now).
9. `POST .../blocks/{block_id}/exercises` — to group.
10. Delete exercise / empty single.

### Phase 4 — blocks: merge, ungroup, patch

11. `POST .../days/{day_id}/merge` — `{ block_ids: [] }`; concatenate instruction.
12. `POST .../blocks/{block_id}/ungroup`.
13. `PATCH .../blocks/{block_id}` — `block_type` (group, not `single`), `instruction`; **single blocks → 400**.

### Phase 5 — move and order

14. Reorder blocks in day; reorder exercises in block.
15. `POST .../blocks/{block_id}/move` — other day (single or group).
16. Extract exercise → `single` in day; move between blocks.

### Phase 6 — contract and QA

17. `api/openapi.yaml` — `ProgramDayBlock`, breaking: day via `blocks`.
18. `internal/apicheck/schema.go`.
19. Update [programs.md](programs.md); publish validation.
20. `make check`; add row to `status.md` → implemented.

## API

Contract: `api/openapi.yaml`. Day = `blocks[]`. Single is a regular block (`block_type: single`); appears as one exercise to users, but API always operates on blocks.

| Operation | API |
| --------- | --- |
| Add exercise to day | `POST .../days/{day_id}/blocks` — always `single` + `exercise` |
| Create group | `POST .../blocks/merge` (2+ blocks) |
| Update exercise | `PUT .../blocks/{block_id}/exercises/{item_id}` |
| Delete exercise | `DELETE .../blocks/{block_id}/exercises/{item_id}` |
| Add to group | `POST .../blocks/{block_id}/exercises` |
| merge / ungroup / move / reorder | see [api-endpoints.md](../../.claude/rules/api-endpoints.md) § Program week subtree; reorder passes full sibling id list |
| Block visibility | `PUT .../blocks/{block_id}/clients` — [program-block-visibility.md](program-block-visibility.md) |

## Out of scope

Structured `settings` per `block_type`; client timers; `weight_kg`; undo merge.

## See also

- [programs.md](programs.md) — programs, publish, versions
- [program-block-visibility.md](program-block-visibility.md) — block visibility by client
- [garbage-cleanup.md](garbage-cleanup.md) — auto-cleanup of `program_block_clients`
