-- name: InsertProgramDraft :one
INSERT INTO mentorix.programs (created_by, modified_by, status, modified_at)
VALUES ($1, $1, $2, $3)
RETURNING id;

-- name: InsertProgramDay :one
INSERT INTO mentorix.program_days (program_id, day_number, sort_order)
VALUES ($1, $2, $3)
RETURNING id;

-- name: GetProgramByID :one
SELECT
  id, created_by, modified_by, status, name, description, category, difficulty,
  preview_image_url, created_at, modified_at, deleted_at
FROM mentorix.programs
WHERE id = $1;

-- name: CountPrograms :one
SELECT COUNT(*)::int AS total
FROM mentorix.programs
WHERE deleted_at IS NULL
  AND (sqlc.narg('filter_created_by')::uuid IS NULL OR created_by = sqlc.narg('filter_created_by'))
  AND (sqlc.narg('q_pattern')::text IS NULL OR name ILIKE sqlc.narg('q_pattern') ESCAPE '\')
  AND (sqlc.narg('filter_statuses')::text[] IS NULL OR status = ANY(sqlc.narg('filter_statuses')))
  AND (sqlc.narg('filter_category')::text IS NULL OR category = sqlc.narg('filter_category'))
  AND (sqlc.narg('filter_difficulty')::text IS NULL OR difficulty = sqlc.narg('filter_difficulty'));

-- name: ListPrograms :many
SELECT
  id, created_by, modified_by, status, name, description, category, difficulty,
  preview_image_url, created_at, modified_at, deleted_at
FROM mentorix.programs
WHERE deleted_at IS NULL
  AND (sqlc.narg('filter_created_by')::uuid IS NULL OR created_by = sqlc.narg('filter_created_by'))
  AND (sqlc.narg('q_pattern')::text IS NULL OR name ILIKE sqlc.narg('q_pattern') ESCAPE '\')
  AND (sqlc.narg('filter_statuses')::text[] IS NULL OR status = ANY(sqlc.narg('filter_statuses')))
  AND (sqlc.narg('filter_category')::text IS NULL OR category = sqlc.narg('filter_category'))
  AND (sqlc.narg('filter_difficulty')::text IS NULL OR difficulty = sqlc.narg('filter_difficulty'))
ORDER BY
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'asc' THEN name END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'desc' THEN name END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'created_at' AND sqlc.arg('sort_order') = 'asc' THEN created_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'created_at' AND sqlc.arg('sort_order') = 'desc' THEN created_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'modified_at' AND sqlc.arg('sort_order') = 'asc' THEN modified_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'modified_at' AND sqlc.arg('sort_order') = 'desc' THEN modified_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'status' AND sqlc.arg('sort_order') = 'asc' THEN status END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'status' AND sqlc.arg('sort_order') = 'desc' THEN status END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'category' AND sqlc.arg('sort_order') = 'asc' THEN category END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'category' AND sqlc.arg('sort_order') = 'desc' THEN category END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'difficulty' AND sqlc.arg('sort_order') = 'asc' THEN difficulty END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'difficulty' AND sqlc.arg('sort_order') = 'desc' THEN difficulty END DESC NULLS LAST,
  id ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProgram :execrows
UPDATE mentorix.programs SET
  modified_by = sqlc.arg('modified_by'),
  modified_at = sqlc.arg('modified_at'),
  name = COALESCE(sqlc.narg('name'), name),
  description = COALESCE(sqlc.narg('description'), description),
  category = COALESCE(sqlc.narg('category'), category),
  difficulty = COALESCE(sqlc.narg('difficulty'), difficulty),
  preview_image_url = COALESCE(sqlc.narg('preview_image_url'), preview_image_url)
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: SetProgramStatus :execrows
UPDATE mentorix.programs SET
  status = $2,
  modified_by = $3,
  modified_at = $4
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteProgram :execrows
UPDATE mentorix.programs SET
  deleted_at = $2,
  modified_by = $3,
  modified_at = $2
WHERE id = $1 AND deleted_at IS NULL;

-- name: NextProgramDayNumbers :one
SELECT
  COALESCE(MAX(day_number), 0) + 1::int AS next_day_number,
  COALESCE(MAX(sort_order), 0) + 1::int AS next_sort_order
FROM mentorix.program_days
WHERE program_id = $1;

-- name: InsertProgramDayAuto :exec
INSERT INTO mentorix.program_days (program_id, day_number, sort_order)
VALUES ($1, $2, $3);

-- name: DeleteProgramDay :execrows
DELETE FROM mentorix.program_days
WHERE id = $1 AND program_id = $2;

-- name: DayBelongsToProgram :one
SELECT EXISTS(
  SELECT 1 FROM mentorix.program_days
  WHERE id = $1 AND program_id = $2
) AS ok;

-- name: ExerciseExists :one
SELECT EXISTS(
  SELECT 1 FROM mentorix.exercises WHERE id = $1
) AS ok;

-- name: ListProgramDays :many
SELECT id, day_number, sort_order, created_at
FROM mentorix.program_days
WHERE program_id = $1
ORDER BY sort_order ASC, day_number ASC;

-- name: ListDayExercises :many
SELECT
  pde.id,
  pde.exercise_id,
  e.name,
  e.name_ru,
  pde.sort_order,
  pde.sets,
  pde.reps,
  pde.weight_kg,
  pde.instruction,
  pde.created_at
FROM mentorix.program_day_exercises pde
JOIN mentorix.exercises e ON e.id = pde.exercise_id
WHERE pde.program_day_id = $1
ORDER BY pde.sort_order ASC, pde.created_at ASC;

-- name: NextDayExerciseSort :one
SELECT COALESCE(MAX(sort_order), 0) + 1::int AS next_sort
FROM mentorix.program_day_exercises
WHERE program_day_id = $1;

-- name: InsertDayExercise :exec
INSERT INTO mentorix.program_day_exercises (
  program_day_id, exercise_id, sort_order, sets, reps, weight_kg, instruction
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: UpdateDayExercise :execrows
UPDATE mentorix.program_day_exercises SET
  exercise_id = $3,
  sets = $4,
  reps = $5,
  weight_kg = $6,
  instruction = $7
WHERE id = $1 AND program_day_id = $2;

-- name: DeleteDayExercise :execrows
DELETE FROM mentorix.program_day_exercises
WHERE id = $1 AND program_day_id = $2;
