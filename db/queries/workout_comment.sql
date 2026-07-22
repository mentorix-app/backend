-- Trainer replies on client workout completions.

-- name: GetCompletionForComment :one
-- Scopes the completion to the trainer/client pair and returns the snapshot
-- fields needed for the Telegram notification text.
SELECT
  id,
  client_user_id,
  week_number,
  day_number,
  program_name,
  program_name_ru,
  result_text
FROM mentorix.client_workout_completions
WHERE id = $1
  AND trainer_id = $2
  AND client_user_id = $3;

-- name: CreateCompletionComment :one
INSERT INTO mentorix.client_workout_completion_comments (
  client_workout_completion_id,
  trainer_id,
  comment_text
) VALUES ($1, $2, $3)
RETURNING id, client_workout_completion_id, trainer_id, comment_text, created_at;

-- name: ListCommentsForCompletions :many
SELECT
  id,
  client_workout_completion_id,
  comment_text,
  created_at
FROM mentorix.client_workout_completion_comments
WHERE client_workout_completion_id = ANY (sqlc.arg('completion_ids')::uuid[])
ORDER BY created_at ASC, id ASC;
