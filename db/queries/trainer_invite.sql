-- name: InsertTrainerInvite :one
INSERT INTO mentorix.trainer_invites (trainer_id, token, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTrainerInviteByTokenForUpdate :one
SELECT *
FROM mentorix.trainer_invites
WHERE token = $1
FOR UPDATE;

-- name: ConsumeTrainerInvite :execrows
UPDATE mentorix.trainer_invites
SET consumed_at = $2,
    consumed_by = $3
WHERE id = $1
  AND consumed_at IS NULL;

-- name: InsertTrainerClient :exec
INSERT INTO mentorix.trainer_clients (trainer_id, client_user_id, status)
VALUES ($1, $2, 'active')
ON CONFLICT (trainer_id, client_user_id) DO NOTHING;

-- name: GetTrainerClient :one
SELECT trainer_id, client_user_id, status, blocked_at, created_at
FROM mentorix.trainer_clients
WHERE trainer_id = $1
  AND client_user_id = $2;

-- name: ListTrainerClients :many
SELECT
  tc.client_user_id,
  tc.status,
  tc.created_at,
  u.display_name,
  pa.id AS assignment_id,
  pa.program_id,
  pa.program_version_id,
  pa.status AS assignment_status,
  pa.assigned_at
FROM mentorix.trainer_clients tc
INNER JOIN mentorix.users u ON u.id = tc.client_user_id
LEFT JOIN mentorix.program_assignments pa
  ON pa.trainer_id = tc.trainer_id
  AND pa.client_user_id = tc.client_user_id
  AND pa.status = 'active'
WHERE tc.trainer_id = $1
ORDER BY tc.created_at DESC;

-- name: GetAuthIdentityUserID :one
SELECT user_id
FROM mentorix.auth_identities
WHERE provider = $1
  AND subject = $2;

-- name: GetTrainerUserDisplayName :one
SELECT u.display_name
FROM mentorix.trainers t
INNER JOIN mentorix.users u ON u.id = t.user_id
WHERE t.id = $1;
