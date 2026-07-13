-- name: InsertProgramVersion :one
INSERT INTO mentorix.program_versions (
  program_id,
  version_number,
  published_at,
  published_by,
  name,
  name_ru,
  description,
  description_ru,
  category,
  difficulty,
  preview_image_url,
  content_fingerprint
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: GetProgramVersionByID :one
SELECT *
FROM mentorix.program_versions
WHERE id = $1;

-- name: GetLatestProgramVersionByProgramID :one
SELECT *
FROM mentorix.program_versions
WHERE program_id = $1
ORDER BY version_number DESC
LIMIT 1;

-- name: GetLatestProgramVersionFingerprint :one
SELECT content_fingerprint
FROM mentorix.program_versions
WHERE program_id = $1
ORDER BY version_number DESC
LIMIT 1;

-- name: GetMaxProgramVersionNumber :one
SELECT COALESCE(MAX(version_number), 0)::int AS max_version_number
FROM mentorix.program_versions
WHERE program_id = $1;

-- name: ListProgramVersionsByProgramID :many
SELECT
  pv.*,
  COUNT(pa.id) FILTER (WHERE pa.status = 'active')::int AS assignment_count
FROM mentorix.program_versions pv
LEFT JOIN mentorix.program_assignments pa ON pa.program_version_id = pv.id
WHERE pv.program_id = $1
GROUP BY pv.id
ORDER BY pv.version_number DESC;

-- name: CountProgramAssignmentsByVersionID :one
SELECT COUNT(*)::int AS total
FROM mentorix.program_assignments
WHERE program_version_id = $1;

-- name: CountProgramVersionsByProgramID :one
SELECT COUNT(*)::int AS total
FROM mentorix.program_versions
WHERE program_id = $1;

-- name: DeleteProgramVersion :execrows
DELETE FROM mentorix.program_versions
WHERE id = $1;

-- name: InsertProgramVersionWeek :one
INSERT INTO mentorix.program_version_weeks (
  program_version_id,
  week_number,
  sort_order
) VALUES ($1, $2, $3)
RETURNING *;

-- name: InsertProgramVersionDay :one
INSERT INTO mentorix.program_version_week_days (
  program_version_id,
  program_version_week_id,
  day_number,
  sort_order,
  day_key
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: InsertProgramVersionDayBlock :one
INSERT INTO mentorix.program_version_week_day_blocks (
  program_version_week_day_id,
  block_type,
  instruction,
  sort_order
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: InsertProgramVersionDayExercise :one
INSERT INTO mentorix.program_version_week_day_block_exercises (
  program_version_week_day_block_id,
  exercise_id,
  sort_order,
  sets,
  reps,
  instruction
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListProgramVersionWeeksByVersionID :many
SELECT *
FROM mentorix.program_version_weeks
WHERE program_version_id = $1
ORDER BY sort_order ASC, week_number ASC;

-- name: ListProgramVersionDaysByVersionID :many
SELECT *
FROM mentorix.program_version_week_days
WHERE program_version_id = $1
ORDER BY sort_order ASC, day_number ASC;

-- name: ListProgramVersionDayBlocksByVersionID :many
SELECT pvb.*
FROM mentorix.program_version_week_day_blocks pvb
JOIN mentorix.program_version_week_days pd ON pd.id = pvb.program_version_week_day_id
WHERE pd.program_version_id = $1
ORDER BY pd.sort_order ASC, pd.day_number ASC, pvb.sort_order ASC;

-- name: ListProgramVersionDayExercisesByVersionID :many
SELECT pde.*
FROM mentorix.program_version_week_day_block_exercises pde
JOIN mentorix.program_version_week_day_blocks pvb ON pvb.id = pde.program_version_week_day_block_id
JOIN mentorix.program_version_week_days pd ON pd.id = pvb.program_version_week_day_id
WHERE pd.program_version_id = $1
ORDER BY pd.sort_order ASC, pd.day_number ASC, pvb.sort_order ASC, pde.sort_order ASC;

-- name: ListProgramVersionDayExercisesWithNamesByVersionID :many
SELECT
  pde.id,
  pde.program_version_week_day_block_id,
  pde.exercise_id,
  pde.sort_order,
  pde.sets,
  pde.reps,
  pde.instruction,
  pde.created_at,
  COALESCE(e.name, '')::text AS exercise_name,
  COALESCE(e.name_ru, '')::text AS exercise_name_ru
FROM mentorix.program_version_week_day_block_exercises pde
JOIN mentorix.program_version_week_day_blocks pvb ON pvb.id = pde.program_version_week_day_block_id
JOIN mentorix.program_version_week_days pd ON pd.id = pvb.program_version_week_day_id
LEFT JOIN mentorix.exercises e ON e.id = pde.exercise_id
WHERE pd.program_version_id = $1
ORDER BY pd.sort_order ASC, pd.day_number ASC, pvb.sort_order ASC, pde.sort_order ASC;
