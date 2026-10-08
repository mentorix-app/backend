CREATE INDEX IF NOT EXISTS program_week_days_program_id_idx
  ON mentorix.program_week_days (program_id);
CREATE INDEX IF NOT EXISTS program_week_days_week_id_idx
  ON mentorix.program_week_days (week_id);
CREATE INDEX IF NOT EXISTS program_weeks_program_id_idx
  ON mentorix.program_weeks (program_id);
CREATE INDEX IF NOT EXISTS program_versions_program_id_idx
  ON mentorix.program_versions (program_id);
CREATE INDEX IF NOT EXISTS program_version_weeks_program_version_id_idx
  ON mentorix.program_version_weeks (program_version_id);
CREATE INDEX IF NOT EXISTS program_version_week_days_program_version_id_idx
  ON mentorix.program_version_week_days (program_version_id);
CREATE INDEX IF NOT EXISTS program_version_week_days_program_version_week_id_idx
  ON mentorix.program_version_week_days (program_version_week_id);
CREATE INDEX IF NOT EXISTS program_assignments_trainer_id_idx
  ON mentorix.program_assignments (trainer_id);
CREATE INDEX IF NOT EXISTS trainer_clients_trainer_id_idx
  ON mentorix.trainer_clients (trainer_id);
CREATE INDEX IF NOT EXISTS client_workout_completions_completion_cycle_id_idx
  ON mentorix.client_workout_completions (completion_cycle_id);
