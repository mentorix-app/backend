-- name: ListProgramBlockClients :many
SELECT block_key, client_user_id
FROM mentorix.program_block_clients
WHERE program_id = $1
ORDER BY block_key, client_user_id;

-- name: DeleteProgramBlockClients :exec
DELETE FROM mentorix.program_block_clients
WHERE program_id = $1 AND block_key = $2;

-- name: InsertProgramBlockClient :exec
INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id, created_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (program_id, block_key, client_user_id) DO NOTHING;

-- name: ListAssignedClientUserIDs :many
SELECT DISTINCT client_user_id
FROM mentorix.program_assignments
WHERE program_id = $1;

-- name: ListAssignedProgramVersionIDs :many
SELECT DISTINCT program_version_id
FROM mentorix.program_assignments
WHERE program_id = $1;

-- name: DeleteProgramBlockClientsForClient :exec
DELETE FROM mentorix.program_block_clients
WHERE program_id = $1 AND client_user_id = $2;

-- name: LockProgramForUpdate :exec
SELECT id
FROM mentorix.programs
WHERE id = $1
FOR UPDATE;
