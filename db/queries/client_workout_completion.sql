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
