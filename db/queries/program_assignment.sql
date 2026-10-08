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
  modified_by,
  completion_cycle_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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
    modified_by = $7,
    completion_cycle_id = $8
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

-- name: ListActiveProgramAssignmentsWithVersion :many
-- Same rows and order as ListActiveProgramAssignmentsByProgramID, with the
-- published_at of the version each assignment is on.
SELECT
  pa.id, pa.program_id, pa.program_version_id, pa.trainer_id, pa.client_user_id,
  pa.status, pa.assigned_at, pa.created_at, pa.completion_cycle_id,
  pv.published_at AS version_published_at
FROM mentorix.program_assignments pa
JOIN mentorix.program_versions pv ON pv.id = pa.program_version_id
WHERE pa.program_id = $1
  AND pa.status = 'active'
ORDER BY pa.assigned_at DESC;

-- name: CountActiveProgramAssignmentsByProgramIDs :many
-- Active assignment counts for a page of programs; a program without active
-- assignments has no row.
SELECT program_id, COUNT(*)::int AS total
FROM mentorix.program_assignments
WHERE program_id = ANY(sqlc.arg('program_ids')::uuid[])
  AND status = 'active'
GROUP BY program_id;

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
