-- name: GetExerciseByID :one
SELECT
  e.id, e.name, e.name_ru, e.created_by, e.modified_by, e.modified_at, e.created_at,
  e.equipment, e.exercise_type, e.muscle_group, e.description, e.description_ru,
  e.difficulty, e.video_url, e.preview_image_url,
  COALESCE(u.display_name, '') AS created_by_name
FROM mentorix.exercises e
JOIN mentorix.users u ON u.id = e.created_by
WHERE e.id = $1 AND e.deleted_at IS NULL;

-- name: CreateExercise :one
INSERT INTO mentorix.exercises (
  name, name_ru, created_by, modified_by, modified_at,
  equipment, exercise_type, muscle_group, description, description_ru,
  difficulty, video_url, preview_image_url
) VALUES (
  $1, $2, $3, $4, $5,
  $6, $7, $8, $9, $10,
  $11, $12, $13
)
RETURNING
  id, name, name_ru, created_by, modified_by, modified_at, created_at,
  equipment, exercise_type, muscle_group, description, description_ru,
  difficulty, video_url, preview_image_url;

-- name: UpdateExercise :execrows
UPDATE mentorix.exercises SET
  name = $2,
  name_ru = $3,
  modified_by = $4,
  modified_at = $5,
  equipment = $6,
  exercise_type = $7,
  muscle_group = $8,
  description = $9,
  description_ru = $10,
  difficulty = $11,
  video_url = $12,
  preview_image_url = $13
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteExercises :execrows
UPDATE mentorix.exercises SET
  deleted_at = $2,
  modified_at = $2,
  modified_by = $3
WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL;

-- name: CountExercises :one
SELECT COUNT(*)::int AS total
FROM mentorix.exercises
WHERE deleted_at IS NULL
  AND (sqlc.narg('q_pattern')::text IS NULL OR (
    name ILIKE sqlc.narg('q_pattern') ESCAPE '\'
    OR name_ru ILIKE sqlc.narg('q_pattern') ESCAPE '\'
  ))
  AND (sqlc.narg('filter_type')::text IS NULL OR exercise_type = sqlc.narg('filter_type'))
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
  e.id, e.name, e.name_ru, e.created_by, e.modified_by, e.modified_at, e.created_at,
  e.equipment, e.exercise_type, e.muscle_group, e.description, e.description_ru,
  e.difficulty, e.video_url, e.preview_image_url,
  COALESCE(u.display_name, '') AS created_by_name
FROM mentorix.exercises e
JOIN mentorix.users u ON u.id = e.created_by
WHERE e.deleted_at IS NULL
  AND (sqlc.narg('q_pattern')::text IS NULL OR (
    e.name ILIKE sqlc.narg('q_pattern') ESCAPE '\'
    OR e.name_ru ILIKE sqlc.narg('q_pattern') ESCAPE '\'
  ))
  AND (sqlc.narg('filter_type')::text IS NULL OR e.exercise_type = sqlc.narg('filter_type'))
  AND (sqlc.narg('filter_muscle_group')::text IS NULL OR e.muscle_group = sqlc.narg('filter_muscle_group'))
  AND (sqlc.narg('filter_difficulty')::text IS NULL OR e.difficulty = sqlc.narg('filter_difficulty'))
  AND (
    sqlc.narg('equipment_is_null')::boolean IS NOT TRUE
    OR e.equipment IS NULL
  )
  AND (
    sqlc.narg('filter_equipment')::text IS NULL
    OR e.equipment = sqlc.narg('filter_equipment')
  )
ORDER BY
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'asc' THEN e.name END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'desc' THEN e.name END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'name_ru' AND sqlc.arg('sort_order') = 'asc' THEN e.name_ru END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'name_ru' AND sqlc.arg('sort_order') = 'desc' THEN e.name_ru END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'created_at' AND sqlc.arg('sort_order') = 'asc' THEN e.created_at END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'created_at' AND sqlc.arg('sort_order') = 'desc' THEN e.created_at END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'modified_at' AND sqlc.arg('sort_order') = 'asc' THEN e.modified_at END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'modified_at' AND sqlc.arg('sort_order') = 'desc' THEN e.modified_at END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'difficulty' AND sqlc.arg('sort_order') = 'asc' THEN e.difficulty END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'difficulty' AND sqlc.arg('sort_order') = 'desc' THEN e.difficulty END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'type' AND sqlc.arg('sort_order') = 'asc' THEN e.exercise_type END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'type' AND sqlc.arg('sort_order') = 'desc' THEN e.exercise_type END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'exercise_type' AND sqlc.arg('sort_order') = 'asc' THEN e.exercise_type END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'exercise_type' AND sqlc.arg('sort_order') = 'desc' THEN e.exercise_type END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'muscle_group' AND sqlc.arg('sort_order') = 'asc' THEN e.muscle_group END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'muscle_group' AND sqlc.arg('sort_order') = 'desc' THEN e.muscle_group END DESC,
  CASE WHEN sqlc.arg('sort_by') = 'equipment' AND sqlc.arg('sort_order') = 'asc' THEN e.equipment END ASC,
  CASE WHEN sqlc.arg('sort_by') = 'equipment' AND sqlc.arg('sort_order') = 'desc' THEN e.equipment END DESC,
  e.id ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
