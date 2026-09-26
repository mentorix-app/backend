# Trainer clients (program assignment)

**Status:** implemented  
**Code:** `internal/trainerclient/`

## Purpose

Assign a frozen program version to a trainer's client.

## Database

Uses `trainer_clients`, `program_assignments`, and `trainers` tables.

## API

Contract: `api/openapi.yaml` defines `GET /trainer/clients/{client_user_id}/program-assignment`, `PUT /trainer/clients/program-assignment`, and `GET /trainer/clients` (list).

`GET /trainer/clients` supports pagination (`page`, `limit`), search by `display_name` (`q`, case-insensitive), and sorting by `sort_by=name|linked_at` and `sort_order=asc|desc` (default `linked_at` descending). Trainers see only their own clients; admins see all clients linked to any trainer (deduplicated by `client_user_id`, picking the most recent `linked_at`). The `admin` role is exclusive and does not combine with `trainer`. The `trainer_user_id` field shows the user id of the selected link's trainer; `trainer_display_name` shows that trainer's name. `last_active_at` is the most recent `completed_at` from the client's `client_workout_completions` across any trainer, or `null` if no workouts. `avatar_url` is a signed proxy URL for the Telegram profile photo (empty if no photo exists).

**Admin:** read-only access to the client list (`GET /trainer/clients`). Program assignment (`PUT …/program-assignment` and `GET …/program-assignment`) requires the `trainer` role (the `admin` and `trainer` roles are now mutually exclusive); admins get `403`. Sync (`POST /programs/{id}/assignments/sync`) requires ownership (`created_by`).

`GET /trainer/clients/{client_user_id}/avatar` proxies the avatar using `exp` and `sig` from `avatar_url`; intended for `<img src>` and requires no Bearer token.

Non-obvious rules:

- One active `program_assignments` row per `(trainer_id, client_user_id)` pair in the database. Reassigning updates it (new `completion_cycle_id` if `program_id` changes); clearing deletes it.
- Plan quota on active clients limits assignment and reassignment (returns `409 quota_exceeded` when exceeded); clearing is always allowed. See [subscriptions.md](subscriptions.md).
- Repeating a `PUT` with the same `program_id` returns `already_assigned` with no database update.
- The version assigned is always the latest frozen version. Clearing uses `program_id: null` in the `PUT`.
- Assignment endpoint: `PUT /trainer/clients/program-assignment` takes `client_user_ids` (1–100) and `program_id`; response lists `assigned`, `cleared`, and `skipped` (same as sync). A single client is sent as an array with one id.
- Each `program_assignment` in the list includes `assignment_id`, `program_id`, `program_version_id`, `assigned_at`, `program_name`, `program_name_ru` (from frozen version), and `is_behind_latest` (can sync if `true`; `assignment_id` goes in the sync request body).
- Syncing active assignments uses program endpoints (`POST .../assignments/sync`).

## See also

- [trainer-invites.md](trainer-invites.md) — inviting and linking clients
- [programs.md](programs.md)
