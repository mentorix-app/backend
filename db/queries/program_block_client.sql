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

-- A rule is orphaned only when its block_key exists in neither the working
-- copy (program_week_day_blocks, joined through program_week_days for the
-- program_id) nor any surviving version of the same program. The version
-- half matters: a block_key removed from the working copy can still live
-- inside a frozen version a client is currently assigned to, and deleting
-- its rule there would wrongly un-restrict the block for that client.

-- name: PurgeOrphanProgramBlockClientsForProgram :execrows
DELETE FROM mentorix.program_block_clients pbc
WHERE pbc.program_id = $1
  AND NOT EXISTS (
    SELECT 1
    FROM mentorix.program_week_day_blocks b
    JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id
    WHERE d.program_id = pbc.program_id AND b.block_key = pbc.block_key
  )
  AND NOT EXISTS (
    SELECT 1
    FROM mentorix.program_version_week_day_blocks vb
    JOIN mentorix.program_version_week_days vd
      ON vd.id = vb.program_version_week_day_id
    JOIN mentorix.program_versions v ON v.id = vd.program_version_id
    WHERE v.program_id = pbc.program_id AND vb.block_key = pbc.block_key
  );

-- name: PurgeOrphanProgramBlockClients :execrows
DELETE FROM mentorix.program_block_clients pbc
WHERE NOT EXISTS (
    SELECT 1
    FROM mentorix.program_week_day_blocks b
    JOIN mentorix.program_week_days d ON d.id = b.program_week_day_id
    WHERE d.program_id = pbc.program_id AND b.block_key = pbc.block_key
  )
  AND NOT EXISTS (
    SELECT 1
    FROM mentorix.program_version_week_day_blocks vb
    JOIN mentorix.program_version_week_days vd
      ON vd.id = vb.program_version_week_day_id
    JOIN mentorix.program_versions v ON v.id = vd.program_version_id
    WHERE v.program_id = pbc.program_id AND vb.block_key = pbc.block_key
  );
