-- Trainer analytics: read-only aggregates over client_workout_completions,
-- program_assignments and program version trees.

-- name: GetClientAnalyticsHeader :one
SELECT
  tc.client_user_id,
  u.display_name,
  u.avatar_file_path,
  tc.status,
  tc.created_at AS linked_at,
  (
    SELECT MAX(cwc.completed_at)
    FROM mentorix.client_workout_completions cwc
    WHERE cwc.client_user_id = tc.client_user_id
  ) AS last_active_at
FROM mentorix.trainer_clients tc
INNER JOIN mentorix.users u ON u.id = tc.client_user_id
WHERE tc.trainer_id = $1
  AND tc.client_user_id = $2;

-- name: GetClientCurrentAssignmentAnalytics :one
SELECT
  pa.program_id,
  pa.program_version_id,
  pa.completion_cycle_id,
  pa.assigned_at,
  pv.name AS program_name,
  pv.name_ru AS program_name_ru,
  (
    pa.program_version_id IS DISTINCT FROM (
      SELECT lpv.id
      FROM mentorix.program_versions lpv
      WHERE lpv.program_id = pa.program_id
      ORDER BY lpv.version_number DESC
      LIMIT 1
    )
  )::boolean AS is_behind_latest
FROM mentorix.program_assignments pa
INNER JOIN mentorix.program_versions pv ON pv.id = pa.program_version_id
WHERE pa.trainer_id = $1
  AND pa.client_user_id = $2
  AND pa.status = 'active';

-- name: ListCycleWeekProgress :many
-- Per week of the assigned version: training days (days with blocks)
-- and how many of them are completed in the given cycle.
SELECT
  w.week_number,
  COUNT(d.id) FILTER (
    WHERE EXISTS (
      SELECT 1
      FROM mentorix.program_version_week_day_blocks b
      WHERE b.program_version_week_day_id = d.id
    )
  )::int AS total_days,
  COUNT(d.id) FILTER (
    WHERE EXISTS (
      SELECT 1
      FROM mentorix.client_workout_completions c
      WHERE c.completion_cycle_id = $2
        AND c.day_key = d.day_key
    )
  )::int AS completed_days
FROM mentorix.program_version_weeks w
LEFT JOIN mentorix.program_version_week_days d ON d.program_version_week_id = w.id
WHERE w.program_version_id = $1
GROUP BY w.week_number
ORDER BY w.week_number ASC;

-- name: GetClientActivityStats :one
SELECT
  COUNT(*)::int AS total_completions,
  COUNT(*) FILTER (WHERE completed_at >= sqlc.arg('since_7d'))::int AS completions_last_7_days,
  COUNT(*) FILTER (WHERE completed_at >= sqlc.arg('since_30d'))::int AS completions_last_30_days,
  MIN(completed_at) AS first_completed_at,
  MAX(completed_at) AS last_completed_at
FROM mentorix.client_workout_completions
WHERE trainer_id = $1
  AND client_user_id = $2;

-- name: ListClientCompletionWeeks :many
-- Distinct ISO weeks (UTC) with at least one completion, newest first (streak input).
-- AT TIME ZONE 'UTC' makes truncation independent of the DB session timezone.
SELECT DISTINCT date_trunc('week', completed_at AT TIME ZONE 'UTC')::timestamp AS week_start
FROM mentorix.client_workout_completions
WHERE trainer_id = $1
  AND client_user_id = $2
ORDER BY week_start DESC
LIMIT 520;

-- name: ListClientProgramActivity :many
-- Lifetime totals per program across all cycles and versions.
-- Names come from the journal snapshot so deleted programs still show up.
SELECT
  program_id,
  (array_agg(program_name ORDER BY completed_at DESC))[1]::text AS program_name,
  (array_agg(program_name_ru ORDER BY completed_at DESC))[1]::text AS program_name_ru,
  COUNT(*)::int AS total_completions,
  MIN(completed_at) AS first_completed_at,
  MAX(completed_at) AS last_completed_at
FROM mentorix.client_workout_completions
WHERE trainer_id = $1
  AND client_user_id = $2
GROUP BY program_id
ORDER BY MAX(completed_at) DESC;

-- name: CountClientCompletions :one
SELECT COUNT(*)::int AS total
FROM mentorix.client_workout_completions
WHERE trainer_id = $1
  AND client_user_id = $2
  AND (sqlc.narg('from_at')::timestamptz IS NULL OR completed_at >= sqlc.narg('from_at'))
  AND (sqlc.narg('to_at')::timestamptz IS NULL OR completed_at < sqlc.narg('to_at'));

-- name: ListClientCompletions :many
SELECT
  cwc.id,
  cwc.completed_at,
  cwc.program_id,
  cwc.program_name,
  cwc.program_name_ru,
  cwc.week_number,
  cwc.day_number,
  cwc.result_text,
  COALESCE(cwc.completion_cycle_id = pa.completion_cycle_id, false)::boolean AS is_current_cycle
FROM mentorix.client_workout_completions cwc
LEFT JOIN mentorix.program_assignments pa
  ON pa.trainer_id = cwc.trainer_id
  AND pa.client_user_id = cwc.client_user_id
  AND pa.status = 'active'
WHERE cwc.trainer_id = $1
  AND cwc.client_user_id = $2
  AND (sqlc.narg('from_at')::timestamptz IS NULL OR cwc.completed_at >= sqlc.narg('from_at'))
  AND (sqlc.narg('to_at')::timestamptz IS NULL OR cwc.completed_at < sqlc.narg('to_at'))
ORDER BY cwc.completed_at DESC, cwc.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListTrainerProgramsAnalytics :many
-- One row per non-deleted program owned by the trainer, with completion aggregates.
WITH assignment_progress AS (
  SELECT
    pa.program_id,
    (
      SELECT COUNT(*)
      FROM mentorix.program_version_week_days d
      WHERE d.program_version_id = pa.program_version_id
        AND EXISTS (
          SELECT 1
          FROM mentorix.program_version_week_day_blocks b
          WHERE b.program_version_week_day_id = d.id
        )
    )::int AS total_days,
    (
      SELECT COUNT(*)
      FROM mentorix.client_workout_completions c
      WHERE c.completion_cycle_id = pa.completion_cycle_id
        AND c.day_key IN (
          SELECT d2.day_key
          FROM mentorix.program_version_week_days d2
          WHERE d2.program_version_id = pa.program_version_id
        )
    )::int AS completed_days
  FROM mentorix.program_assignments pa
  WHERE pa.trainer_id = sqlc.arg('trainer_id')
    AND pa.status = 'active'
),
completion_totals AS (
  SELECT
    cwc.program_id,
    COUNT(*)::int AS total_completions,
    COUNT(*) FILTER (WHERE cwc.completed_at >= sqlc.arg('since_30d'))::int AS completions_last_30_days,
    MAX(cwc.completed_at) AS last_activity_at
  FROM mentorix.client_workout_completions cwc
  WHERE cwc.trainer_id = sqlc.arg('trainer_id')
    AND cwc.program_id IS NOT NULL
  GROUP BY cwc.program_id
)
SELECT
  p.id AS program_id,
  p.name,
  p.name_ru,
  p.status,
  (
    SELECT COUNT(DISTINCT d.id)
    FROM mentorix.program_week_days d
    JOIN mentorix.program_weeks w ON w.id = d.week_id
    WHERE w.program_id = p.id
      AND EXISTS (
        SELECT 1
        FROM mentorix.program_week_day_blocks b
        WHERE b.program_week_day_id = d.id
      )
  )::int AS training_days_count,
  COALESCE(ap.active_clients_count, 0)::int AS active_clients_count,
  COALESCE(ct.total_completions, 0)::int AS total_completions,
  COALESCE(ct.completions_last_30_days, 0)::int AS completions_last_30_days,
  COALESCE(ap.avg_completion_percent, 0)::float AS avg_completion_percent,
  ct.last_activity_at
FROM mentorix.programs p
LEFT JOIN (
  SELECT
    program_id,
    COUNT(*)::int AS active_clients_count,
    AVG(
      LEAST(completed_days::float / NULLIF(total_days, 0), 1.0) * 100.0
    )::float AS avg_completion_percent
  FROM assignment_progress
  GROUP BY program_id
) ap ON ap.program_id = p.id
LEFT JOIN completion_totals ct ON ct.program_id = p.id
WHERE p.created_by = sqlc.arg('created_by')
  AND p.deleted_at IS NULL
ORDER BY
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'asc' THEN p.name END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'desc' THEN p.name END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'last_activity' AND sqlc.arg('sort_order') = 'asc' THEN ct.last_activity_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'last_activity' AND sqlc.arg('sort_order') = 'desc' THEN ct.last_activity_at END DESC NULLS LAST,
  p.id ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetProgramAnalyticsHeader :one
SELECT
  p.id AS program_id,
  p.created_by,
  p.name,
  p.name_ru,
  p.status,
  (
    SELECT COALESCE(MAX(pv.version_number), 0)
    FROM mentorix.program_versions pv
    WHERE pv.program_id = p.id
  )::int AS latest_version_number,
  (
    SELECT COUNT(DISTINCT d.id)
    FROM mentorix.program_week_days d
    JOIN mentorix.program_weeks w ON w.id = d.week_id
    WHERE w.program_id = p.id
      AND EXISTS (
        SELECT 1
        FROM mentorix.program_week_day_blocks b
        WHERE b.program_week_day_id = d.id
      )
  )::int AS training_days_count
FROM mentorix.programs p
WHERE p.id = $1
  AND p.deleted_at IS NULL;

-- name: GetProgramCompletionTotals :one
SELECT
  COUNT(*)::int AS total_completions,
  COUNT(*) FILTER (WHERE completed_at >= sqlc.arg('since_30d'))::int AS completions_last_30_days,
  MAX(completed_at) AS last_activity_at
FROM mentorix.client_workout_completions
WHERE trainer_id = $1
  AND program_id = $2;

-- name: ListProgramAnalyticsClients :many
-- Active assignees of the program with per-cycle progress.
SELECT
  pa.client_user_id,
  u.display_name,
  u.avatar_file_path,
  pa.assigned_at,
  (
    pa.program_version_id IS DISTINCT FROM (
      SELECT lpv.id
      FROM mentorix.program_versions lpv
      WHERE lpv.program_id = pa.program_id
      ORDER BY lpv.version_number DESC
      LIMIT 1
    )
  )::boolean AS is_behind_latest,
  (
    SELECT COUNT(*)
    FROM mentorix.program_version_week_days d
    WHERE d.program_version_id = pa.program_version_id
      AND EXISTS (
        SELECT 1
        FROM mentorix.program_version_week_day_blocks b
        WHERE b.program_version_week_day_id = d.id
      )
  )::int AS total_training_days,
  (
    SELECT COUNT(*)
    FROM mentorix.client_workout_completions c
    WHERE c.completion_cycle_id = pa.completion_cycle_id
      AND c.day_key IN (
        SELECT d2.day_key
        FROM mentorix.program_version_week_days d2
        WHERE d2.program_version_id = pa.program_version_id
      )
  )::int AS completed_days,
  (
    SELECT MAX(c.completed_at)
    FROM mentorix.client_workout_completions c
    WHERE c.trainer_id = pa.trainer_id
      AND c.client_user_id = pa.client_user_id
      AND c.program_id = pa.program_id
  ) AS last_completed_at
FROM mentorix.program_assignments pa
INNER JOIN mentorix.users u ON u.id = pa.client_user_id
WHERE pa.program_id = $1
  AND pa.status = 'active'
ORDER BY u.display_name ASC, pa.client_user_id ASC;

-- name: ListProgramAnalyticsWeeks :many
-- Drop-off by program week: current-cycle completions of active assignees.
SELECT
  c.week_number,
  COUNT(*)::int AS completions_count,
  COUNT(DISTINCT c.client_user_id)::int AS distinct_clients_count
FROM mentorix.client_workout_completions c
INNER JOIN mentorix.program_assignments pa
  ON pa.completion_cycle_id = c.completion_cycle_id
  AND pa.status = 'active'
WHERE pa.program_id = $1
GROUP BY c.week_number
ORDER BY c.week_number ASC;
