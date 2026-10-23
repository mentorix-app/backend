-- name: InsertUser :one
INSERT INTO mentorix.users (primary_email, display_name)
VALUES ($1, $2)
RETURNING id;

-- name: InsertAuthIdentityIfAbsent :execrows
INSERT INTO mentorix.auth_identities (user_id, provider, subject)
VALUES ($1, $2, $3)
ON CONFLICT ON CONSTRAINT auth_identities_provider_subject_uniq DO NOTHING;

-- name: DeleteUserWithoutIdentity :exec
DELETE FROM mentorix.users u
WHERE u.id = $1
  AND NOT EXISTS (SELECT 1 FROM mentorix.auth_identities i WHERE i.user_id = u.id);

-- name: InsertUserRole :exec
INSERT INTO mentorix.user_roles (user_id, role)
VALUES ($1, $2);

-- name: InsertTrainerIfAbsent :exec
INSERT INTO mentorix.trainers (user_id)
VALUES ($1)
ON CONFLICT (user_id) DO NOTHING;

-- name: AcceptTrainerTerms :exec
-- A repeat of the same version changes nothing, so the first acceptance keeps its time.
UPDATE mentorix.trainers
SET terms_version = sqlc.arg('terms_version')::text,
    terms_accepted_at = now()
WHERE user_id = sqlc.arg('user_id')
  AND terms_version IS DISTINCT FROM sqlc.arg('terms_version')::text;

-- name: LockTrainerByUserID :one
-- Quota writers lock the trainers row and then touch the user row through a foreign key,
-- so a transaction that locks both takes the trainers row first.
SELECT id
FROM mentorix.trainers
WHERE user_id = $1
FOR UPDATE;

-- name: LockTrainerInvitesByUserID :many
-- Accepting an invite locks the invite row (GetTrainerInviteByTokenForUpdate) and, for a new
-- link, then the trainers row (subscription.LockTrainer). The account deletion locks the
-- invites first, in this order, so it does not wait for an accept the other way round.
-- The lock covers the rows that exist when the query runs, not invites created later.
SELECT i.id
FROM mentorix.trainer_invites i
WHERE i.trainer_id = (SELECT t.id FROM mentorix.trainers t WHERE t.user_id = sqlc.arg('user_id'))
ORDER BY i.id
FOR UPDATE;

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
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshSessions :exec
UPDATE mentorix.auth_refresh_sessions
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: PurgeStaleRefreshSessions :execrows
DELETE FROM mentorix.auth_refresh_sessions
WHERE (revoked_at IS NOT NULL AND revoked_at < now() - interval '30 days')
   OR (expires_at < now() - interval '30 days');

-- name: UserHasTrainerLink :one
SELECT EXISTS (
  SELECT 1 FROM mentorix.trainer_clients WHERE client_user_id = $1
);

-- name: MoveSignInIdentitiesToUser :execrows
UPDATE mentorix.auth_identities
SET user_id = sqlc.arg('to_user_id')
WHERE user_id = sqlc.arg('from_user_id')
  AND provider = ANY (sqlc.arg('providers')::text[]);

-- name: FillUserPrimaryEmailIfEmpty :exec
UPDATE mentorix.users u
SET primary_email = src.primary_email
FROM mentorix.users src
WHERE u.id = sqlc.arg('user_id')
  AND src.id = sqlc.arg('from_user_id')
  AND (u.primary_email IS NULL OR u.primary_email = '')
  AND src.primary_email IS NOT NULL
  AND src.primary_email <> '';

-- name: FillUserDisplayNameIfEmpty :execrows
UPDATE mentorix.users
SET display_name = sqlc.arg('display_name')
WHERE id = sqlc.arg('user_id')
  AND btrim(display_name) = '';

-- name: SetUserPrimaryEmailIfEmpty :execrows
UPDATE mentorix.users
SET primary_email = sqlc.arg('primary_email')::text
WHERE id = sqlc.arg('user_id')
  AND (primary_email IS NULL OR primary_email = '');

-- name: DeleteUserByID :execrows
DELETE FROM mentorix.users
WHERE id = $1;

-- name: CountUserIdentitiesOutsideProviders :one
SELECT count(*)
FROM mentorix.auth_identities
WHERE user_id = sqlc.arg('user_id')
  AND NOT (provider = ANY (sqlc.arg('providers')::text[]));

-- name: LockUserForUpdate :one
-- FOR UPDATE is deliberate: FOR NO KEY UPDATE does not conflict with the key-share
-- lock that a foreign key check takes on the user row, so a concurrent insert of a
-- row that references the user would not wait.
SELECT id
FROM mentorix.users
WHERE id = $1
FOR UPDATE;

-- name: ListAuthIdentitiesByUserID :many
SELECT provider, subject
FROM mentorix.auth_identities
WHERE user_id = $1
ORDER BY provider, subject;

-- name: TrainerHasBusinessData :one
-- True when the user's trainer side holds anything that account deletion must not destroy:
-- clients, authored or last edited programs, program trees, versions and exercises,
-- assignments, workout completions and comments on them, block visibility rules, and
-- entitlements paid through a store. The edited columns are the ones the database cannot
-- detach from the user (RESTRICT, or SET NULL on a NOT NULL column). Invites and
-- admin-granted entitlements do not count: they go with the trainers row.
-- What the trainer deleted does not count either. A purgeable program is a soft-deleted
-- program the user created that no assignment or workout completion references, itself or
-- through one of its versions. A purgeable exercise is a soft-deleted exercise owned by the
-- user's trainers row that no program or version tree outside the purgeable programs uses.
-- PurgeDeletedProgramsByCreator and PurgeDeletedExercisesByOwner remove them. A purgeable
-- program is exempt from every column the user holds in its own rows and trees. An exercise
-- the user created or last edited that the user's trainers row does not own (a global one
-- included) still counts, and so does a soft-deleted program that another trainer created
-- and the user last edited.
WITH trainer AS (
  SELECT t.id FROM mentorix.trainers t WHERE t.user_id = sqlc.arg('user_id')
),
purgeable_programs AS (
  SELECT p.id
  FROM mentorix.programs p
  WHERE p.created_by = sqlc.arg('user_id')
    AND p.deleted_at IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM mentorix.program_assignments pa WHERE pa.program_id = p.id)
    AND NOT EXISTS (
      SELECT 1
      FROM mentorix.program_assignments pa
      JOIN mentorix.program_versions pv ON pv.id = pa.program_version_id
      WHERE pv.program_id = p.id
    )
    AND NOT EXISTS (SELECT 1 FROM mentorix.client_workout_completions cwc WHERE cwc.program_id = p.id)
    AND NOT EXISTS (
      SELECT 1
      FROM mentorix.client_workout_completions cwc
      JOIN mentorix.program_versions pv ON pv.id = cwc.program_version_id
      WHERE pv.program_id = p.id
    )
),
purgeable_exercises AS (
  SELECT e.id
  FROM mentorix.exercises e
  WHERE e.owner_trainer_id IN (SELECT id FROM trainer)
    AND e.deleted_at IS NOT NULL
    AND NOT EXISTS (
      SELECT 1
      FROM mentorix.program_week_day_block_exercises pwdbe
      JOIN mentorix.program_week_day_blocks pwdb ON pwdb.id = pwdbe.program_week_day_block_id
      JOIN mentorix.program_week_days pwd ON pwd.id = pwdb.program_week_day_id
      WHERE pwdbe.exercise_id = e.id
        AND NOT EXISTS (SELECT 1 FROM purgeable_programs pp WHERE pp.id = pwd.program_id)
    )
    AND NOT EXISTS (
      SELECT 1
      FROM mentorix.program_version_week_day_block_exercises pvwdbe
      JOIN mentorix.program_version_week_day_blocks pvwdb ON pvwdb.id = pvwdbe.program_version_week_day_block_id
      JOIN mentorix.program_version_week_days pvwd ON pvwd.id = pvwdb.program_version_week_day_id
      JOIN mentorix.program_versions pv ON pv.id = pvwd.program_version_id
      WHERE pvwdbe.exercise_id = e.id
        AND NOT EXISTS (SELECT 1 FROM purgeable_programs pp WHERE pp.id = pv.program_id)
    )
)
SELECT (
  EXISTS (SELECT 1 FROM mentorix.trainer_clients tc WHERE tc.trainer_id IN (SELECT id FROM trainer))
  OR EXISTS (
    SELECT 1 FROM mentorix.programs p
    WHERE (p.created_by = sqlc.arg('user_id') OR p.modified_by = sqlc.arg('user_id'))
      AND NOT EXISTS (SELECT 1 FROM purgeable_programs pp WHERE pp.id = p.id)
  )
  OR EXISTS (
    SELECT 1 FROM mentorix.exercises e
    WHERE (e.created_by = sqlc.arg('user_id') OR e.modified_by = sqlc.arg('user_id')
        OR e.owner_trainer_id IN (SELECT id FROM trainer))
      AND NOT EXISTS (SELECT 1 FROM purgeable_exercises pe WHERE pe.id = e.id)
  )
  OR EXISTS (
    SELECT 1 FROM mentorix.program_weeks pw
    WHERE pw.modified_by = sqlc.arg('user_id')
      AND NOT EXISTS (SELECT 1 FROM purgeable_programs pp WHERE pp.id = pw.program_id)
  )
  OR EXISTS (
    SELECT 1 FROM mentorix.program_week_days pwd
    WHERE pwd.modified_by = sqlc.arg('user_id')
      AND NOT EXISTS (SELECT 1 FROM purgeable_programs pp WHERE pp.id = pwd.program_id)
  )
  OR EXISTS (
    SELECT 1 FROM mentorix.program_week_day_block_exercises pwdbe
    WHERE pwdbe.modified_by = sqlc.arg('user_id')
      AND NOT EXISTS (
        SELECT 1
        FROM mentorix.program_week_day_blocks pwdb
        JOIN mentorix.program_week_days pwd ON pwd.id = pwdb.program_week_day_id
        JOIN purgeable_programs pp ON pp.id = pwd.program_id
        WHERE pwdb.id = pwdbe.program_week_day_block_id
      )
  )
  OR EXISTS (
    SELECT 1 FROM mentorix.program_versions pv
    WHERE pv.published_by = sqlc.arg('user_id')
      AND NOT EXISTS (SELECT 1 FROM purgeable_programs pp WHERE pp.id = pv.program_id)
  )
  OR EXISTS (
    SELECT 1 FROM mentorix.program_assignments pa
    WHERE pa.created_by = sqlc.arg('user_id') OR pa.modified_by = sqlc.arg('user_id')
      OR pa.trainer_id IN (SELECT id FROM trainer)
  )
  OR EXISTS (SELECT 1 FROM mentorix.client_workout_completions cwc WHERE cwc.trainer_id IN (SELECT id FROM trainer))
  OR EXISTS (SELECT 1 FROM mentorix.client_workout_completion_comments cwcc WHERE cwcc.trainer_id IN (SELECT id FROM trainer))
  OR EXISTS (
    SELECT 1 FROM mentorix.program_block_clients pbc
    WHERE pbc.created_by = sqlc.arg('user_id')
      AND NOT EXISTS (SELECT 1 FROM purgeable_programs pp WHERE pp.id = pbc.program_id)
  )
  OR EXISTS (
    SELECT 1 FROM mentorix.trainer_plan_entitlements tpe
    WHERE tpe.source IN ('app_store', 'google_play') AND tpe.trainer_id IN (SELECT id FROM trainer)
  )
)::boolean AS has_business_data;

-- name: PurgeDeletedProgramsByCreator :execrows
-- Removes the soft-deleted programs the user created, with their trees, versions and block
-- visibility rules (the foreign keys cascade). The deletion calls it after
-- TrainerHasBusinessData passed under the account locks, so every such program is purgeable.
-- As a safety net, an assignment that still references a purged program makes the delete
-- fail on the foreign key (RESTRICT, the deletion answers 409 and changes nothing). A workout
-- completion the app writes carries the account's trainers row, so it cannot appear before
-- the deletion commits; its insert waits for the deletion's lock.
DELETE FROM mentorix.programs
WHERE created_by = sqlc.arg('user_id')
  AND deleted_at IS NOT NULL;

-- name: PurgeDeletedExercisesByOwner :execrows
-- Removes the soft-deleted exercises owned by the user's trainers row. Run after
-- PurgeDeletedProgramsByCreator: a program tree that still uses an exercise makes the delete
-- fail on the foreign key.
DELETE FROM mentorix.exercises
WHERE owner_trainer_id = (SELECT t.id FROM mentorix.trainers t WHERE t.user_id = sqlc.arg('user_id'))
  AND deleted_at IS NOT NULL;
