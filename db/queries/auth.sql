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

-- name: InsertTrainerIfMissing :exec
INSERT INTO mentorix.trainers (user_id)
VALUES ($1)
ON CONFLICT (user_id) DO NOTHING;

-- name: GetUserIDByPrimaryEmail :one
SELECT id
FROM mentorix.users
WHERE lower(primary_email) = lower(sqlc.arg(email)::text)
LIMIT 1;

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

-- name: InsertRefreshSessionInFamily :exec
INSERT INTO mentorix.auth_refresh_sessions (user_id, family_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetRefreshSessionForRotation :one
-- Reads the row in any state and locks it. It runs after the family lock, so every time
-- comparison uses statement_timestamp(): now() is the start of the transaction, before the lock wait.
SELECT
  user_id,
  family_id,
  (expires_at <= statement_timestamp())::boolean AS expired,
  (revoked_at IS NOT NULL)::boolean AS revoked,
  (rotated_at IS NOT NULL)::boolean AS rotated,
  (rotated_at IS NOT NULL
    AND statement_timestamp() - rotated_at <= make_interval(secs => sqlc.arg(grace_seconds)::float8))::boolean AS within_grace
FROM mentorix.auth_refresh_sessions
WHERE token_hash = sqlc.arg(token_hash)
FOR UPDATE;

-- name: GetRefreshSessionFamilyID :one
-- Unlocked read: it only finds the family to lock before the row is read again.
SELECT family_id
FROM mentorix.auth_refresh_sessions
WHERE token_hash = $1;

-- name: SetLocalLockTimeout :exec
SELECT set_config('lock_timeout', sqlc.arg(timeout)::text, true);

-- name: ListUserLiveRefreshFamilies :many
SELECT DISTINCT family_id
FROM mentorix.auth_refresh_sessions
WHERE user_id = $1 AND revoked_at IS NULL
ORDER BY family_id;

-- name: LockRefreshFamily :exec
-- Serializes every rotation and revoke of one family until the transaction ends.
SELECT pg_advisory_xact_lock(hashtextextended($1::uuid::text, 0));

-- name: MarkRefreshSessionRotated :exec
UPDATE mentorix.auth_refresh_sessions
SET rotated_at = statement_timestamp(), revoked_at = statement_timestamp()
WHERE token_hash = $1;

-- name: RevokeRefreshSessionByHash :exec
UPDATE mentorix.auth_refresh_sessions
SET revoked_at = statement_timestamp()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: MarkRefreshFamilyRotated :exec
-- Spends the family's unrevoked row, expired or not, so the successor is its only live token.
-- The partial unique index allows at most one such row.
UPDATE mentorix.auth_refresh_sessions
SET rotated_at = statement_timestamp(), revoked_at = statement_timestamp()
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: RefreshFamilyHasActiveSession :one
SELECT EXISTS (
  SELECT 1
  FROM mentorix.auth_refresh_sessions
  WHERE family_id = $1 AND revoked_at IS NULL AND rotated_at IS NULL AND expires_at > statement_timestamp()
)::boolean;

-- name: RevokeRefreshFamily :execrows
UPDATE mentorix.auth_refresh_sessions
SET revoked_at = statement_timestamp()
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshSessions :exec
UPDATE mentorix.auth_refresh_sessions
SET revoked_at = statement_timestamp()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: PurgeStaleRefreshSessions :execrows
DELETE FROM mentorix.auth_refresh_sessions
WHERE (revoked_at IS NOT NULL AND revoked_at < now() - interval '30 days')
   OR (expires_at < now() - interval '30 days');

-- name: ListUserSignInMethods :many
SELECT DISTINCT provider
FROM mentorix.auth_identities
WHERE user_id = $1
ORDER BY provider;

-- name: GetEmailPasswordHashByUserID :one
-- No row means the user does not exist; an empty hash means no password identity.
SELECT COALESCE(i.password_hash, '')::text AS password_hash
FROM mentorix.users u
LEFT JOIN mentorix.auth_identities i
  ON i.user_id = u.id AND i.provider = sqlc.arg(provider)::text
WHERE u.id = sqlc.arg(user_id)
ORDER BY (i.password_hash IS NULL), i.created_at
LIMIT 1;
