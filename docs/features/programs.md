# Programs

**Status:** implemented  
**Code:** `internal/program/`

## Purpose

Trainer program templates: weeks → days → blocks → exercises; publish creates frozen `program_versions`.

## Database

`programs`, `program_weeks`, `program_week_days`, `program_week_day_blocks`, `program_week_day_block_exercises`, `program_versions` (+ version tree), `program_assignments`.

## API

Contract: `api/openapi.yaml` (Programs tag).

Rules that are not obvious:

- `POST /programs` → `draft`, empty `name`, week 1 with **7 empty days** automatically.
- `PUT …/reorder` (weeks, days, blocks, exercises in block): body is **complete** ordered sibling id list (each exactly once); partial list → 400 `invalid reorder: … count mismatch`.
- Statuses: `draft` → `published` → `archived`; no `published` → `draft`.
- Publish from `archived` — status change only, no new version.
- Publish (draft): validate name/category/difficulty, ≥1 week; each non-empty day has blocks with exercises (`single`: 1 exercise; group: ≥1); `sets`/`reps` optional (`null`/omit); when present, string format: digits only (`3`), single `/` (`5/4`), or single `-` (`3-6`).
- `training_days_count` in `Program` / `ProgramDetail`: count of days with ≥1 block or exercise (empty days don't count); in list from SQL, in `GET /programs/{id}` from loaded weeks.
- Published: in-place edit + `has_unpublished_changes`; publish-update → new version; discard-unpublished → revert working copy to latest version (clients and frozen versions unchanged; `day_key` preserved, week/day/block ids new).
- Trainer sees their own; admin sees all, **read-only**: any mutation by non-owner → `403`.
- Plan quota on active programs (draft+published): create draft and re-publish from archive → `409 quota_exceeded` if limit filled; mutations of existing blocked on exceeded (read-only after downgrade); archive/delete always allowed. See [subscriptions.md](subscriptions.md).
- `POST /programs/{id}/assignments/sync` — owner only (`created_by`); admin without ownership → `403` (like assign). After sync — auto-cleanup unused versions.

## See also

- [program-blocks.md](program-blocks.md) — blocks in day
- [trainer-clients.md](trainer-clients.md) — client assignment
