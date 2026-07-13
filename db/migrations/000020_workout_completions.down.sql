DROP TABLE IF EXISTS mentorix.client_workout_completions;

ALTER TABLE mentorix.program_assignments
  DROP COLUMN IF EXISTS completion_cycle_id;

ALTER TABLE mentorix.program_version_week_days
  DROP CONSTRAINT IF EXISTS program_version_week_days_version_day_key_uniq;

ALTER TABLE mentorix.program_version_week_days
  DROP COLUMN IF EXISTS day_key;

ALTER TABLE mentorix.program_week_days
  DROP CONSTRAINT IF EXISTS program_week_days_program_id_day_key_uniq;

ALTER TABLE mentorix.program_week_days
  DROP COLUMN IF EXISTS day_key;
