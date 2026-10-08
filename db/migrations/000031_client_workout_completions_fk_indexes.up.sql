-- Fail fast instead of queueing behind a long transaction and stalling every query after it.
SET LOCAL lock_timeout = '3s';

-- These foreign keys are ON DELETE SET NULL and had no index, so deleting an assignment
-- or a version scanned the whole table.
CREATE INDEX IF NOT EXISTS client_workout_completions_program_assignment_id_idx
  ON mentorix.client_workout_completions (program_assignment_id);
CREATE INDEX IF NOT EXISTS client_workout_completions_program_version_id_idx
  ON mentorix.client_workout_completions (program_version_id);
CREATE INDEX IF NOT EXISTS client_workout_completions_program_id_idx
  ON mentorix.client_workout_completions (program_id);
