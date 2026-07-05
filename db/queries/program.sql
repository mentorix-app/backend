-- name: InsertProgramDraft :one
INSERT INTO mentorix.programs (created_by, modified_by, status, modified_at)
VALUES ($1, $1, $2, $3)
RETURNING id;

-- name: InsertProgramWeek :one
INSERT INTO mentorix.program_weeks (program_id, week_number, sort_order, modified_at, modified_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: InsertProgramDay :one
INSERT INTO mentorix.program_week_days (program_id, week_id, day_number, sort_order)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: GetProgramByID :one
SELECT
  p.id, p.created_by, p.modified_by, p.status, p.name, p.name_ru, p.description, p.description_ru,
  p.category, p.difficulty, p.preview_image_url, p.created_at, p.modified_at, p.deleted_at,
  COALESCE(u.display_name, '') AS created_by_name
FROM mentorix.programs p
JOIN mentorix.users u ON u.id = p.created_by
WHERE p.id = $1;

-- name: CountPrograms :one
SELECT COUNT(*)::int AS total
FROM mentorix.programs
WHERE deleted_at IS NULL
  AND (sqlc.narg('filter_created_by')::uuid IS NULL OR created_by = sqlc.narg('filter_created_by'))
  AND (
    sqlc.narg('q_pattern')::text IS NULL
    OR name ILIKE sqlc.narg('q_pattern') ESCAPE '\'
    OR name_ru ILIKE sqlc.narg('q_pattern') ESCAPE '\'
  )
  AND (sqlc.narg('filter_statuses')::text[] IS NULL OR status = ANY(sqlc.narg('filter_statuses')))
  AND (sqlc.narg('filter_category')::text IS NULL OR category = sqlc.narg('filter_category'))
  AND (sqlc.narg('filter_difficulty')::text IS NULL OR difficulty = sqlc.narg('filter_difficulty'));

-- name: ListPrograms :many
SELECT
  p.id, p.created_by, p.modified_by, p.status, p.name, p.name_ru, p.description, p.description_ru,
  p.category, p.difficulty, p.preview_image_url, p.created_at, p.modified_at, p.deleted_at,
  COALESCE(u.display_name, '') AS created_by_name
FROM mentorix.programs p
JOIN mentorix.users u ON u.id = p.created_by
WHERE p.deleted_at IS NULL
  AND (sqlc.narg('filter_created_by')::uuid IS NULL OR p.created_by = sqlc.narg('filter_created_by'))
  AND (
    sqlc.narg('q_pattern')::text IS NULL
    OR p.name ILIKE sqlc.narg('q_pattern') ESCAPE '\'
    OR p.name_ru ILIKE sqlc.narg('q_pattern') ESCAPE '\'
  )
  AND (sqlc.narg('filter_statuses')::text[] IS NULL OR p.status = ANY(sqlc.narg('filter_statuses')))
  AND (sqlc.narg('filter_category')::text IS NULL OR p.category = sqlc.narg('filter_category'))
  AND (sqlc.narg('filter_difficulty')::text IS NULL OR p.difficulty = sqlc.narg('filter_difficulty'))
ORDER BY
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'asc' THEN p.name END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'name' AND sqlc.arg('sort_order') = 'desc' THEN p.name END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'name_ru' AND sqlc.arg('sort_order') = 'asc' THEN p.name_ru END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'name_ru' AND sqlc.arg('sort_order') = 'desc' THEN p.name_ru END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'created_at' AND sqlc.arg('sort_order') = 'asc' THEN p.created_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'created_at' AND sqlc.arg('sort_order') = 'desc' THEN p.created_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'modified_at' AND sqlc.arg('sort_order') = 'asc' THEN p.modified_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'modified_at' AND sqlc.arg('sort_order') = 'desc' THEN p.modified_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'status' AND sqlc.arg('sort_order') = 'asc' THEN p.status END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'status' AND sqlc.arg('sort_order') = 'desc' THEN p.status END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'category' AND sqlc.arg('sort_order') = 'asc' THEN p.category END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'category' AND sqlc.arg('sort_order') = 'desc' THEN p.category END DESC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'difficulty' AND sqlc.arg('sort_order') = 'asc' THEN p.difficulty END ASC NULLS LAST,
  CASE WHEN sqlc.arg('sort_by') = 'difficulty' AND sqlc.arg('sort_order') = 'desc' THEN p.difficulty END DESC NULLS LAST,
  p.id ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProgram :execrows
UPDATE mentorix.programs SET
  modified_by = sqlc.arg('modified_by'),
  modified_at = sqlc.arg('modified_at'),
  name = COALESCE(sqlc.narg('name'), name),
  name_ru = COALESCE(sqlc.narg('name_ru'), name_ru),
  description = COALESCE(sqlc.narg('description'), description),
  description_ru = COALESCE(sqlc.narg('description_ru'), description_ru),
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

-- name: NextProgramWeekNumbers :one
SELECT
  COALESCE(MAX(week_number), 0) + 1::int AS next_week_number,
  COALESCE(MAX(sort_order), 0) + 1::int AS next_sort_order
FROM mentorix.program_weeks
WHERE program_id = $1;

-- name: ListProgramWeeks :many
SELECT id, week_number, sort_order, created_at
FROM mentorix.program_weeks
WHERE program_id = $1
ORDER BY sort_order ASC, week_number ASC;

-- name: CountProgramWeeks :one
SELECT COUNT(*)::int AS count
FROM mentorix.program_weeks
WHERE program_id = $1;

-- name: DeleteProgramWeek :execrows
DELETE FROM mentorix.program_weeks
WHERE id = $1 AND program_id = $2;

-- name: WeekBelongsToProgram :one
SELECT EXISTS(
  SELECT 1 FROM mentorix.program_weeks
  WHERE id = $1 AND program_id = $2
) AS ok;

-- name: NextProgramDayNumbersForWeek :one
SELECT
  COALESCE(MAX(day_number), 0) + 1::int AS next_day_number,
  COALESCE(MAX(sort_order), 0) + 1::int AS next_sort_order
FROM mentorix.program_week_days
WHERE week_id = $1;

-- name: InsertProgramDayAuto :exec
INSERT INTO mentorix.program_week_days (program_id, week_id, day_number, sort_order)
VALUES ($1, $2, $3, $4);

-- name: DeleteProgramDay :execrows
DELETE FROM mentorix.program_week_days
WHERE id = $1 AND program_id = $2 AND week_id = $3;

-- name: DayBelongsToProgram :one
SELECT EXISTS(
  SELECT 1 FROM mentorix.program_week_days
  WHERE id = $1 AND program_id = $2
) AS ok;

-- name: DayBelongsToWeek :one
SELECT EXISTS(
  SELECT 1 FROM mentorix.program_week_days
  WHERE id = $1 AND week_id = $2 AND program_id = $3
) AS ok;

-- name: CountProgramDaysForWeek :one
SELECT COUNT(*)::int AS count
FROM mentorix.program_week_days
WHERE week_id = $1;

-- name: ExerciseExists :one
SELECT EXISTS(
  SELECT 1 FROM mentorix.exercises WHERE id = $1 AND deleted_at IS NULL
) AS ok;

-- name: ListProgramDaysForWeek :many
SELECT id, day_number, sort_order, created_at
FROM mentorix.program_week_days
WHERE week_id = $1
ORDER BY sort_order ASC, day_number ASC;

-- name: ListDayBlocks :many
SELECT id, block_type, instruction, sort_order, created_at
FROM mentorix.program_week_day_blocks
WHERE program_week_day_id = $1
ORDER BY sort_order ASC, created_at ASC;

-- name: GetDayBlockByID :one
SELECT id, program_week_day_id, block_type, instruction, sort_order, created_at
FROM mentorix.program_week_day_blocks
WHERE id = $1;

-- name: BlockBelongsToDay :one
SELECT EXISTS(
  SELECT 1 FROM mentorix.program_week_day_blocks
  WHERE id = $1 AND program_week_day_id = $2
) AS ok;

-- name: BlockBelongsToProgram :one
SELECT EXISTS(
  SELECT 1
  FROM mentorix.program_week_day_blocks pdb
  JOIN mentorix.program_week_days pd ON pd.id = pdb.program_week_day_id
  WHERE pdb.id = $1 AND pd.program_id = $2
) AS ok;

-- name: NextDayBlockSort :one
SELECT COALESCE(MAX(sort_order), 0) + 1::int AS next_sort
FROM mentorix.program_week_day_blocks
WHERE program_week_day_id = $1;

-- name: InsertDayBlock :one
INSERT INTO mentorix.program_week_day_blocks (
  program_week_day_id, block_type, instruction, sort_order
) VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: UpdateDayBlock :execrows
UPDATE mentorix.program_week_day_blocks SET
  block_type = COALESCE(sqlc.narg('block_type'), block_type),
  instruction = COALESCE(sqlc.narg('instruction'), instruction)
WHERE id = sqlc.arg('id') AND program_week_day_id = sqlc.arg('program_week_day_id');

-- name: DeleteDayBlock :execrows
DELETE FROM mentorix.program_week_day_blocks
WHERE id = $1 AND program_week_day_id = $2;

-- name: UpdateDayBlockOrder :exec
UPDATE mentorix.program_week_day_blocks SET
  sort_order = $3
WHERE id = $1 AND program_week_day_id = $2;

-- name: UpdateDayBlockPlacement :exec
UPDATE mentorix.program_week_day_blocks SET
  program_week_day_id = $2,
  sort_order = $3
WHERE id = $1;

-- name: ListDayBlockIDsForDay :many
SELECT id
FROM mentorix.program_week_day_blocks
WHERE program_week_day_id = $1
ORDER BY sort_order ASC, created_at ASC;

-- name: ListBlockExercises :many
SELECT
  pde.id,
  pde.exercise_id,
  e.name,
  e.name_ru,
  pde.sort_order,
  pde.sets,
  pde.reps,
  pde.instruction,
  pde.created_at
FROM mentorix.program_week_day_block_exercises pde
JOIN mentorix.exercises e ON e.id = pde.exercise_id
WHERE pde.program_week_day_block_id = $1
ORDER BY pde.sort_order ASC, pde.created_at ASC;

-- name: NextBlockExerciseSort :one
SELECT COALESCE(MAX(sort_order), 0) + 1::int AS next_sort
FROM mentorix.program_week_day_block_exercises
WHERE program_week_day_block_id = $1;

-- name: InsertBlockExercise :exec
INSERT INTO mentorix.program_week_day_block_exercises (
  program_week_day_block_id, exercise_id, sort_order, sets, reps, instruction,
  modified_at, modified_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateBlockExercise :execrows
UPDATE mentorix.program_week_day_block_exercises SET
  exercise_id = $3,
  sets = $4,
  reps = $5,
  instruction = $6,
  modified_at = $7,
  modified_by = $8
WHERE id = $1 AND program_week_day_block_id = $2;

-- name: DeleteBlockExercise :execrows
DELETE FROM mentorix.program_week_day_block_exercises
WHERE id = $1 AND program_week_day_block_id = $2;

-- name: UpdateBlockExercisePlacement :exec
UPDATE mentorix.program_week_day_block_exercises SET
  program_week_day_block_id = $2,
  sort_order = $3,
  modified_at = $4,
  modified_by = $5
WHERE id = $1;

-- name: CountBlockExercises :one
SELECT COUNT(*)::int AS count
FROM mentorix.program_week_day_block_exercises
WHERE program_week_day_block_id = $1;

-- name: ListExerciseItemsByWeek :many
SELECT pde.id, pdb.program_week_day_id
FROM mentorix.program_week_day_block_exercises pde
JOIN mentorix.program_week_day_blocks pdb ON pdb.id = pde.program_week_day_block_id
JOIN mentorix.program_week_days pd ON pd.id = pdb.program_week_day_id
WHERE pd.week_id = $1;

-- name: ListBlocksByWeek :many
SELECT pdb.id, pdb.program_week_day_id, pdb.block_type, pdb.instruction, pdb.sort_order, pdb.created_at
FROM mentorix.program_week_day_blocks pdb
JOIN mentorix.program_week_days pd ON pd.id = pdb.program_week_day_id
WHERE pd.week_id = $1
ORDER BY pd.sort_order ASC, pd.day_number ASC, pdb.sort_order ASC, pdb.created_at ASC;

-- name: GetBlockExerciseMeta :one
SELECT pde.id, pde.program_week_day_block_id, pdb.program_week_day_id, pdb.block_type
FROM mentorix.program_week_day_block_exercises pde
JOIN mentorix.program_week_day_blocks pdb ON pdb.id = pde.program_week_day_block_id
WHERE pde.id = $1;

-- name: UpdateProgramWeekOrder :exec
UPDATE mentorix.program_weeks SET
  sort_order = $3,
  week_number = $4
WHERE id = $1 AND program_id = $2;

-- name: UpdateProgramDayOrder :exec
UPDATE mentorix.program_week_days SET
  sort_order = $3,
  day_number = $4
WHERE id = $1 AND week_id = $2;

-- name: ListProgramWeekIDs :many
SELECT id
FROM mentorix.program_weeks
WHERE program_id = $1
ORDER BY sort_order ASC, week_number ASC;

-- name: ListProgramDayIDsForWeek :many
SELECT id
FROM mentorix.program_week_days
WHERE week_id = $1
ORDER BY sort_order ASC, day_number ASC;
