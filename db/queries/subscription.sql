-- name: ListActiveTrainerPlanEntitlements :many
SELECT id, trainer_id, plan_code, source, status, valid_from, valid_until, created_at, modified_at
FROM mentorix.trainer_plan_entitlements
WHERE trainer_id = $1
  AND status = 'active'
  AND (valid_until IS NULL OR valid_until > now());

-- name: LockTrainerRow :one
SELECT id
FROM mentorix.trainers
WHERE id = $1
FOR UPDATE;

-- name: GetTrainerUsage :one
SELECT
  (
    SELECT COUNT(*) FROM mentorix.exercises e
    WHERE e.owner_trainer_id = t.id AND e.deleted_at IS NULL
  )::int AS exercises,
  (
    SELECT COUNT(*) FROM mentorix.programs p
    WHERE p.created_by = t.user_id
      AND p.status IN ('draft', 'published')
      AND p.deleted_at IS NULL
  )::int AS active_programs,
  (
    SELECT COUNT(*) FROM mentorix.trainer_clients tc
    WHERE tc.trainer_id = t.id AND tc.status = 'active'
  )::int AS active_clients
FROM mentorix.trainers t
WHERE t.id = $1;

-- name: UpsertAdminPlanGrant :one
INSERT INTO mentorix.trainer_plan_entitlements (trainer_id, plan_code, source, status, valid_until)
VALUES ($1, $2, 'admin', 'active', NULL)
ON CONFLICT (trainer_id) WHERE status = 'active' AND source = 'admin'
DO UPDATE SET plan_code = EXCLUDED.plan_code, modified_at = now()
RETURNING id, trainer_id, plan_code, source, status, valid_from, valid_until, created_at, modified_at;

-- name: RevokeAdminPlanGrant :execrows
UPDATE mentorix.trainer_plan_entitlements
SET status = 'revoked', modified_at = now()
WHERE trainer_id = $1 AND source = 'admin' AND status = 'active';
