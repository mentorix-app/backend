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

-- name: CopyProgramBlockClients :exec
INSERT INTO mentorix.program_block_clients (program_id, block_key, client_user_id, created_by)
SELECT src.program_id, sqlc.arg('target_block_key'), src.client_user_id, src.created_by
FROM mentorix.program_block_clients src
WHERE src.program_id = sqlc.arg('program_id')
  AND src.block_key = sqlc.arg('source_block_key')
ON CONFLICT (program_id, block_key, client_user_id) DO NOTHING;
