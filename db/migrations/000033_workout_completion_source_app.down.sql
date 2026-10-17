SET LOCAL lock_timeout = '3s';

-- Fails while rows with source 'app' exist: the log is never rewritten to fit the old check.
-- The failure rolls the statements back but leaves golang-migrate marking the database
-- dirty at version 32 while the schema is still at 33. Recovery: `migrate force 33`.
ALTER TABLE mentorix.client_workout_completions
  DROP CONSTRAINT client_workout_completions_source_check;

ALTER TABLE mentorix.client_workout_completions
  ADD CONSTRAINT client_workout_completions_source_check CHECK (source = 'telegram');
