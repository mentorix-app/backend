-- name: InsertProgramAssignment :one
INSERT INTO mentorix.program_assignments (
  program_id,
  program_version_id,
  trainer_id,
  client_user_id,
  status,
  assigned_at,
  created_by,
  modified_at,
  modified_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetProgramAssignmentByID :one
SELECT *
FROM mentorix.program_assignments
WHERE id = $1;

-- name: GetProgramAssignmentByTrainerClient :one
SELECT *
FROM mentorix.program_assignments
WHERE trainer_id = $1
  AND client_user_id = $2;

-- name: GetActiveProgramAssignmentByTrainerClient :one
SELECT *
FROM mentorix.program_assignments
WHERE trainer_id = $1
  AND client_user_id = $2
  AND status = 'active';

-- name: UpdateProgramAssignment :one
UPDATE mentorix.program_assignments
SET program_id = $3,
    program_version_id = $4,
    assigned_at = $5,
    modified_at = $6,
    modified_by = $7
WHERE trainer_id = $1
  AND client_user_id = $2
RETURNING *;

-- name: DeleteProgramAssignmentByTrainerClient :execrows
DELETE FROM mentorix.program_assignments
WHERE trainer_id = $1
  AND client_user_id = $2;

-- name: DeleteProgramAssignmentsByProgramID :execrows
DELETE FROM mentorix.program_assignments
WHERE program_id = $1;

-- name: ListProgramAssignmentsByProgramID :many
SELECT *
FROM mentorix.program_assignments
WHERE program_id = $1
ORDER BY assigned_at DESC;

-- name: ListActiveProgramAssignmentsByProgramID :many
SELECT *
FROM mentorix.program_assignments
WHERE program_id = $1
  AND status = 'active'
ORDER BY assigned_at DESC;

-- name: CountActiveProgramAssignmentsByProgramID :one
SELECT COUNT(*)::int AS total
FROM mentorix.program_assignments
WHERE program_id = $1
  AND status = 'active';

-- name: UpdateProgramAssignmentVersion :execrows
UPDATE mentorix.program_assignments
SET program_version_id = $2,
    modified_at = $3,
    modified_by = $4
WHERE id = $1
  AND status = 'active';

-- name: CountActiveProgramAssignmentsByVersionID :one
SELECT COUNT(*)::int AS total
FROM mentorix.program_assignments
WHERE program_version_id = $1
  AND status = 'active';

-- name: TrainerClientLinkActive :one
SELECT status
FROM mentorix.trainer_clients
WHERE trainer_id = $1
  AND client_user_id = $2;

-- name: GetTrainerIDByUserID :one
SELECT id
FROM mentorix.trainers
WHERE user_id = $1;
