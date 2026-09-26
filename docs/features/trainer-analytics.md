# Trainer analytics

**Status:** implemented  
**Code:** `internal/analytics/`

## Purpose

Two analytics screens for trainers: one per client (card, current program progress, activity, workout log) and one per program (list with summaries plus detailed breakdown for one program). Read-only; data comes from the `client_workout_completions` log and active `program_assignments`.

## Database

Uses `client_workout_completions`, `program_assignments`, `program_versions`, `program_version_week_days`, `program_version_week_day_blocks` (progress denominator), `trainer_clients`, and `programs`. Queries are in `db/queries/analytics.sql`. No new tables or migrations.

## API

Contract: `api/openapi.yaml` defines `GET /trainer/clients/{client_user_id}/analytics`, `GET /trainer/clients/{client_user_id}/completions`, `GET /trainer/programs/analytics`, `GET /trainer/programs/{program_id}/analytics`, and `GET /trainer/programs/{program_id}/weeks/{week_number}/results`. All require the `trainer` role (returns `403` for admin).

Access control: the client must be in the trainer's `trainer_clients` list, and the program must be `created_by` that trainer; otherwise returns `404`.

Non-obvious rules:

- **Current program progress** is tracked by the assignment's `completion_cycle_id` against the assigned frozen version's days. A cycle survives version syncs, so progress spans all versions in one assignment; changing `program_id` creates a new cycle.
- **Workout days** (progress denominator) are days in the version that contain at least one block. A day is marked done if its `day_key` appears in the cycle's log; completed days removed from the current version do not count toward percentage. `completion_percent` is capped at 100 with one decimal place.
- **Client activity** (`activity`) aggregates across all cycles and programs for that trainer (the log is never deleted). `by_program` sums activity over all time for each program, using the name from the log snapshot (deleted programs appear with `program_id: null`).
- **`week_streak`** counts consecutive calendar weeks (Monday–Sunday, UTC) with ≥1 workout. The current incomplete week does not break the streak if it has no workouts yet.
- **Completions log** (`/completions`) supports pagination and `from`/`to` parameters (RFC3339 or `YYYY-MM-DD`, `to` exclusive). Results sort by `completed_at` descending. `is_current_cycle` indicates whether a completion belongs to the current assignment. `items[].comments` returns trainer replies (see [workout-comments.md](workout-comments.md)).
- **Program list** includes all non-deleted programs the trainer created (including drafts). `total_completions` sums all log entries by `program_id` across all versions and cycles. `avg_completion_percent` averages progress across active assignments; `null` if no clients. Sorting options are `sort_by=name|last_activity` (default `last_activity` descending).
- **Program details** show `clients` (active assignments with current cycle progress, unpaginated but limited by client quota) and `weeks` (submissions per `week_number` for drop-off analysis). `avatar_url` follows the same format as `GET /trainer/clients`.
- **Week results matrix** (`/programs/{program_id}/weeks/{week_number}/results`) displays clients × workout days in the selected week. Columns are days with blocks in the **latest** version; cells fill based on `day_key` from each assignment's current cycle (`submitted` or `no_result` with `result_text` and `comments`). A week with no workout days returns `404`. Summary includes slots, submissions, missing, percentage, and `behind_clients_count`.

## See also

- [workout-completions.md](workout-completions.md) — log and cycle/day_key rules
- [trainer-clients.md](trainer-clients.md) — client list and assignments
- [programs.md](programs.md) — versions and sync
- [client-analytics.md](client-analytics.md) — same summary from client perspective
