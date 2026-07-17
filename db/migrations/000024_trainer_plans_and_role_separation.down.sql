-- Role/data changes (4a-4g) are not reversible; hard-deleted trainer data
-- can only be restored from a backup taken before the up migration.

DROP TRIGGER IF EXISTS user_roles_admin_exclusive_trg ON mentorix.user_roles;
DROP FUNCTION IF EXISTS mentorix.user_roles_admin_exclusive();

DROP INDEX IF EXISTS mentorix.trainer_clients_trainer_id_active_idx;
DROP INDEX IF EXISTS mentorix.programs_created_by_status_idx;

DROP TABLE IF EXISTS mentorix.trainer_plan_entitlements;

DROP INDEX IF EXISTS mentorix.exercises_owner_trainer_id_idx;
ALTER TABLE mentorix.exercises DROP COLUMN IF EXISTS owner_trainer_id;
