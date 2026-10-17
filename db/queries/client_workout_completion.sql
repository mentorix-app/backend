-- name: InsertClientWorkoutCompletion :one
INSERT INTO mentorix.client_workout_completions (
  client_user_id,
  trainer_id,
  completed_at,
  program_id,
  program_version_id,
  program_assignment_id,
  completion_cycle_id,
  day_key,
  week_number,
  day_number,
  program_name,
  program_name_ru,
  day_snapshot,
  result_text,
  source
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
RETURNING *;

-- name: InsertClientWorkoutCompletionForAssignment :one
-- Writes the completion only while the assignment still has the cycle, version, client
-- and trainer the caller saw and the link is active. No matching row inserts nothing and
-- returns no row. The program comes from the row.
-- No lock on the assignment row: the trainer's reassignment and account deletion lock
-- the trainer or user row first and the assignment second, and a share lock taken here
-- before the foreign key checks made the two wait for each other (deadlock). A mark that
-- runs at the same moment as a reassignment may land in the old cycle; a request after
-- the reassignment committed never matches.
INSERT INTO mentorix.client_workout_completions (
  client_user_id,
  trainer_id,
  completed_at,
  program_id,
  program_version_id,
  program_assignment_id,
  completion_cycle_id,
  day_key,
  week_number,
  day_number,
  program_name,
  program_name_ru,
  day_snapshot,
  result_text,
  source
)
SELECT
  pa.client_user_id,
  pa.trainer_id,
  sqlc.arg(completed_at)::timestamptz,
  pa.program_id,
  pa.program_version_id,
  pa.id,
  pa.completion_cycle_id,
  sqlc.arg(day_key)::uuid,
  sqlc.arg(week_number)::int,
  sqlc.arg(day_number)::int,
  sqlc.arg(program_name)::text,
  sqlc.arg(program_name_ru)::text,
  sqlc.arg(day_snapshot)::jsonb,
  sqlc.arg(result_text)::text,
  sqlc.arg(source)::text
FROM mentorix.program_assignments pa
WHERE pa.id = sqlc.arg(program_assignment_id)::uuid
  AND pa.client_user_id = sqlc.arg(client_user_id)::uuid
  AND pa.trainer_id = sqlc.arg(trainer_id)::uuid
  AND pa.program_version_id = sqlc.arg(program_version_id)::uuid
  AND pa.completion_cycle_id = sqlc.arg(completion_cycle_id)::uuid
  AND EXISTS (
    SELECT 1
    FROM mentorix.trainer_clients tc
    WHERE tc.trainer_id = pa.trainer_id
      AND tc.client_user_id = pa.client_user_id
      AND tc.status = 'active'
  )
RETURNING *;

-- name: ClientWorkoutCompletionExists :one
SELECT EXISTS(
  SELECT 1
  FROM mentorix.client_workout_completions
  WHERE completion_cycle_id = $1
    AND day_key = $2
) AS ok;

-- name: ListCompletedDayKeysByCycle :many
SELECT day_key
FROM mentorix.client_workout_completions
WHERE completion_cycle_id = $1;

-- name: DeleteClientWorkoutCompletionsByClient :execrows
-- The trainer's comments on the deleted completions go with them (ON DELETE CASCADE).
DELETE FROM mentorix.client_workout_completions
WHERE client_user_id = $1;
