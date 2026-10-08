-- Fail fast instead of queueing behind a long transaction and stalling every query after it.
SET LOCAL lock_timeout = '3s';

-- Each index below is the leading column of a unique or primary-key index on the same
-- table, so the unique index already serves its lookups and the extra one only slows writes.
DROP INDEX IF EXISTS mentorix.program_week_days_program_id_idx;
DROP INDEX IF EXISTS mentorix.program_week_days_week_id_idx;
DROP INDEX IF EXISTS mentorix.program_weeks_program_id_idx;
DROP INDEX IF EXISTS mentorix.program_versions_program_id_idx;
DROP INDEX IF EXISTS mentorix.program_version_weeks_program_version_id_idx;
DROP INDEX IF EXISTS mentorix.program_version_week_days_program_version_id_idx;
DROP INDEX IF EXISTS mentorix.program_version_week_days_program_version_week_id_idx;
DROP INDEX IF EXISTS mentorix.program_assignments_trainer_id_idx;
DROP INDEX IF EXISTS mentorix.trainer_clients_trainer_id_idx;
DROP INDEX IF EXISTS mentorix.client_workout_completions_completion_cycle_id_idx;
