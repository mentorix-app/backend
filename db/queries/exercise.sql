-- name: GetExerciseByID :one
SELECT
  id, name, name_ru, added_by, modified_by, modified_at, created_at,
  equipment, type, muscle_group, description, description_ru,
  difficulty, video_url, preview_image_url
FROM mentorix.exercises
WHERE id = $1;

-- name: CreateExercise :one
INSERT INTO mentorix.exercises (
  name, name_ru, added_by, modified_by, modified_at,
  equipment, type, muscle_group, description, description_ru,
  difficulty, video_url, preview_image_url
) VALUES (
  $1, $2, $3, $4, $5,
  $6, $7, $8, $9, $10,
  $11, $12, $13
)
RETURNING
  id, name, name_ru, added_by, modified_by, modified_at, created_at,
  equipment, type, muscle_group, description, description_ru,
  difficulty, video_url, preview_image_url;

-- name: UpdateExercise :execrows
UPDATE mentorix.exercises SET
  name = $2,
  name_ru = $3,
  modified_by = $4,
  modified_at = $5,
  equipment = $6,
  type = $7,
  muscle_group = $8,
  description = $9,
  description_ru = $10,
  difficulty = $11,
  video_url = $12,
  preview_image_url = $13
WHERE id = $1;

-- name: DeleteExercises :execrows
DELETE FROM mentorix.exercises
WHERE id = ANY($1::uuid[]);

-- name: CountExercises :one
SELECT COUNT(*)::int AS total
FROM mentorix.exercises
WHERE
  (sqlc.narg('q_pattern')::text IS NULL OR (
    name ILIKE sqlc.narg('q_pattern') ESCAPE '\'
    OR name_ru ILIKE sqlc.narg('q_pattern') ESCAPE '\'
  ))
  AND (sqlc.narg('filter_type')::text IS NULL OR type = sqlc.narg('filter_type'))
  AND (sqlc.narg('filter_muscle_group')::text IS NULL OR muscle_group = sqlc.narg('filter_muscle_group'))
  AND (sqlc.narg('filter_difficulty')::text IS NULL OR difficulty = sqlc.narg('filter_difficulty'))
  AND (
    sqlc.narg('equipment_is_null')::boolean IS NOT TRUE
    OR equipment IS NULL
  )
  AND (
    sqlc.narg('filter_equipment')::text IS NULL
    OR equipment = sqlc.narg('filter_equipment')
  );

-- name: ListExercises :many
SELECT
  id, name, name_ru, added_by, modified_by, modified_at, created_at,
  equipment, type, muscle_group, description, description_ru,
  difficulty, video_url, preview_image_url
FROM mentorix.exercises
WHERE
  (sqlc.narg('q_pattern')::text IS NULL OR (
    name ILIKE sqlc.narg('q_pattern') ESCAPE '\'
    OR name_ru ILIKE sqlc.narg('q_pattern') ESCAPE '\'
  ))
  AND (sqlc.narg('filter_type')::text IS NULL OR type = sqlc.narg('filter_type'))
  AND (sqlc.narg('filter_muscle_group')::text IS NULL OR muscle_group = sqlc.narg('filter_muscle_group'))
  AND (sqlc.narg('filter_difficulty')::text IS NULL OR difficulty = sqlc.narg('filter_difficulty'))
  AND (
    sqlc.narg('equipment_is_null')::boolean IS NOT TRUE
    OR equipment IS NULL
  )
  AND (
    sqlc.narg('filter_equipment')::text IS NULL
    OR equipment = sqlc.narg('filter_equipment')
  )
ORDER BY
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'asc' THEN name END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'desc' THEN name END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'name_ru' AND sqlc.arg('sort_order') = 'asc' THEN name_ru END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'name_ru' AND sqlc.arg('sort_order') = 'desc' THEN name_ru END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'created_at' AND sqlc.arg('sort_order') = 'asc' THEN created_at END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'created_at' AND sqlc.arg('sort_order') = 'desc' THEN created_at END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'modified_at' AND sqlc.arg('sort_order') = 'asc' THEN modified_at END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'modified_at' AND sqlc.arg('sort_order') = 'desc' THEN modified_at END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'difficulty' AND sqlc.arg('sort_order') = 'asc' THEN difficulty END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'difficulty' AND sqlc.arg('sort_order') = 'desc' THEN difficulty END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'type' AND sqlc.arg('sort_order') = 'asc' THEN type END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'type' AND sqlc.arg('sort_order') = 'desc' THEN type END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'muscle_group' AND sqlc.arg('sort_order') = 'asc' THEN muscle_group END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'muscle_group' AND sqlc.arg('sort_order') = 'desc' THEN muscle_group END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'equipment' AND sqlc.arg('sort_order') = 'asc' THEN equipment END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'equipment' AND sqlc.arg('sort_order') = 'desc' THEN equipment END DESC,
  id ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
