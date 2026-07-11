-- name: InsertUser :one
INSERT INTO mentorix.users (primary_email, display_name)
VALUES ($1, $2)
RETURNING id;

-- name: InsertAuthIdentity :exec
INSERT INTO mentorix.auth_identities (user_id, provider, subject, password_hash)
VALUES ($1, $2, $3, $4);

-- name: InsertUserRole :exec
INSERT INTO mentorix.user_roles (user_id, role)
VALUES ($1, $2);

-- name: InsertTrainer :exec
INSERT INTO mentorix.trainers (user_id)
VALUES ($1);

-- name: GetUserByID :one
SELECT
  COALESCE(primary_email, '') AS primary_email,
  display_name,
  created_at
FROM mentorix.users
WHERE id = $1;

-- name: UpdateUserDisplayName :execrows
UPDATE mentorix.users
SET display_name = $2
WHERE id = $1;

-- name: UpdateUserAvatarFilePath :exec
UPDATE mentorix.users
SET avatar_file_path = $2
WHERE id = $1;

-- name: GetUserAvatarFilePath :one
SELECT avatar_file_path
FROM mentorix.users
WHERE id = $1;

-- name: ListUserRoles :many
SELECT role
FROM mentorix.user_roles
WHERE user_id = $1
ORDER BY role;

-- name: GrantUserRole :exec
INSERT INTO mentorix.user_roles (user_id, role)
VALUES ($1, $2)
ON CONFLICT (user_id, role) DO NOTHING;

-- name: GetEmailPasswordIdentity :one
SELECT user_id, COALESCE(password_hash, '') AS password_hash
FROM mentorix.auth_identities
WHERE provider = $1 AND subject = $2;

-- name: InsertRefreshSession :exec
INSERT INTO mentorix.auth_refresh_sessions (user_id, token_hash, expires_at)
VALUES ($1, $2, $3);

-- name: GetRefreshSessionUserForUpdate :one
SELECT user_id
FROM mentorix.auth_refresh_sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
FOR UPDATE;

-- name: RevokeRefreshSessionByHash :exec
UPDATE mentorix.auth_refresh_sessions
SET revoked_at = now()
WHERE token_hash = $1;

-- name: RevokeAllUserRefreshSessions :exec
UPDATE mentorix.auth_refresh_sessions
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: PurgeStaleRefreshSessions :execrows
DELETE FROM mentorix.auth_refresh_sessions
WHERE (revoked_at IS NOT NULL AND revoked_at < now() - interval '30 days')
   OR (expires_at < now() - interval '30 days');
