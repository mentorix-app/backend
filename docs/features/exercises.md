# Exercises

**Status:** implemented  
**Code:** `internal/exercise/`

## Purpose

Exercise catalog: global (admin, shared for all) and private (trainer-owned, under plan quota).

## Database

`exercises` (soft delete: `deleted_at`; `owner_trainer_id NULL` = global, otherwise owner is trainer).

## API

Contract: `api/openapi.yaml` (Exercises tag).

Rules that are not obvious:

- Access: `trainer` or `admin`. Admin: sees all, CRUD only global. Trainer: sees global + own, CRUD only own; create/edit under plan quota (`409 quota_exceeded`, see [subscriptions.md](subscriptions.md)); delete always allowed.
- Response includes `scope` (`global|private`) and `owner_user_id`; list filter `?scope=`.
- Trainer can add global and **own** exercises to a program; other trainers' private exercises → 400. Existing links are grandfathered — check only on add/replace.
- Enum in database/API: `snake_case` (`exercise_type`).
- `video_url` — optional; if set, only HTTPS URL to YouTube (`youtube.com`, `youtu.be`: `/watch`, `/embed`, `/shorts`, `/live`).

## See also

- [subscriptions.md](subscriptions.md) — quotas
- `.claude/rules/database-naming.md`
