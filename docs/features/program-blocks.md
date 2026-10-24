# Program blocks

**Status:** implemented  
**Code:** `internal/program/` (extended)  
**Related:** [programs.md](programs.md)

## Purpose

A day in a program contains an ordered list of **blocks**. A single is one exercise; a group is a block with `block_type` ≠ `single`, created by merge or by copying a group.

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
| `block_type` | text + CHECK | not `type`; values below |
| `instruction` | text DEFAULT `''` | |
| `sort_order` | int | position in day |
| `rounds` | int NULL | 1..100 |
| `duration_sec` | int NULL | 1..86400 |
| `time_cap_sec` | int NULL | 1..86400 |
| `work_sec` | int NULL | 1..86400 |
| `rest_sec` | int NULL | 0..86400 |
| `created_at` | timestamptz | |
| `modified_at` | timestamptz | |
| `modified_by` | uuid FK → `users` | nullable |
| `block_key` | uuid NOT NULL DEFAULT `gen_random_uuid()` | stable block identity across publish and discard |

`block_type` values: `single`, `emom`, `amrap`, `for_time`, `intervals`, `chipper`, `superset`, `skill_work`, `strength`, `conditioning`, `gymnastics`, `weightlifting`.

The five format settings are nullable and also exist on the version tree. The rules table in [`block_type.go`](../../internal/program/block_type.go) says which type takes which:

- `emom`, `superset`: `rounds`
- `amrap`: `duration_sec`
- `for_time`: `rounds`, `time_cap_sec`
- `intervals`: `rounds`, `work_sec`, `rest_sec`
- other types: none

`emom`, `amrap`, `for_time`, `intervals`, `chipper` and `superset` carry no `sets`.

### `program_week_day_block_exercises`

- FK: `program_week_day_block_id` → `program_week_day_blocks`.
- Fields: `exercise_id`, `sort_order`, `sets`, `reps`, `instruction`, `created_at`.

Mirror for version tree (see naming rule). Publish copies blocks with their settings and `block_key` into the version tree.

### `program_block_clients` — block visibility

Who sees the block; table schema, `block_key`, day invariant, and inheritance during merge/ungroup/extract — [program-block-visibility.md](program-block-visibility.md).

**`sort_order`:** after any operation touching order (create, move, reorder, delete, merge, ungroup, extract), the store normalizes siblings to unique `1..N` without gaps. Response from `GET /programs/{id}` returns the tree already sorted by `sort_order`.

## Behavior

| Action | Result |
| ------ | ------ |
| Add exercise to day (catalog, multiselect) | one `single` block per exercise (`POST .../days/{day_id}/blocks`) |
| Add exercise to group | `POST .../blocks/{block_id}/exercises` (group exists after merge) |
| Create group | only via `POST .../blocks/merge` (2+ blocks); default `skill_work`, no format settings; instruction concatenates instruction from participating **groups** (`\n\n`, empty ones skipped); singles only → `""`; **400** (`ErrValidation`) if participants have different client lists (including "all shared" ≠ "one limited") — no single correct result, merge rejected |
| Patch group (`PATCH .../blocks/{block_id}`) | each setting: absent keeps, `null` clears, integer sets; a value for a setting the resulting type does not take is **400**; settings the new type does not take are reset, `rounds` carries over; a PATCH of a block whose resulting type has no sets clears the sets of its exercises (a type change included) |
| Sets in a type without sets | add or update exercise with `sets` set (non-null) is **400**; moving an exercise into such a block clears its `sets`; publish adds no rule |
| Ungroup | each exercise becomes `single`, inherits group client list |
| Duplicate exercise (`POST .../exercises/{item_id}/duplicate`) | group: the copy sits right after the original; `single`: a new `single` block right after it, with its own copy of the client rules |
| Copy block (`POST .../blocks/{block_id}/copy`) | any block type, with settings and exercises, appended to the end of any day of the same program (the source day included); new `block_key`, client rules copied as separate rows; no day-invariant check, publish validates it |
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

Client timers; `weight_kg`; undo merge.

## See also

- [programs.md](programs.md) — programs, publish, versions
- [program-block-visibility.md](program-block-visibility.md) — block visibility by client
- [garbage-cleanup.md](garbage-cleanup.md) — auto-cleanup of `program_block_clients`
