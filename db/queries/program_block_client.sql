-- name: ListProgramBlockClients :many
SELECT block_key, client_user_id
FROM mentorix.program_block_clients
WHERE program_id = $1
ORDER BY block_key, client_user_id;
