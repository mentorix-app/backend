DROP INDEX IF EXISTS mentorix.program_assignments_client_user_id_idx;
DROP INDEX IF EXISTS mentorix.program_assignments_trainer_id_idx;
DROP INDEX IF EXISTS mentorix.program_assignments_program_version_id_idx;
DROP INDEX IF EXISTS mentorix.program_assignments_program_id_idx;
DROP INDEX IF EXISTS mentorix.program_assignments_trainer_client_active_uniq;
DROP TABLE IF EXISTS mentorix.program_assignments;

DROP INDEX IF EXISTS mentorix.program_version_day_exercises_program_version_day_id_idx;
DROP TABLE IF EXISTS mentorix.program_version_day_exercises;

DROP INDEX IF EXISTS mentorix.program_version_days_program_version_week_id_idx;
DROP INDEX IF EXISTS mentorix.program_version_days_program_version_id_idx;
DROP TABLE IF EXISTS mentorix.program_version_days;

DROP INDEX IF EXISTS mentorix.program_version_weeks_program_version_id_idx;
DROP TABLE IF EXISTS mentorix.program_version_weeks;

DROP INDEX IF EXISTS mentorix.program_versions_program_id_idx;
DROP TABLE IF EXISTS mentorix.program_versions;
