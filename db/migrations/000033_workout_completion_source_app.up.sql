-- Fail fast instead of queueing behind a long transaction and stalling every query after it.
SET LOCAL lock_timeout = '3s';

-- The app marks workout days too. The set only grows, so the running code stays valid
-- before and after this change.
ALTER TABLE mentorix.client_workout_completions
  DROP CONSTRAINT client_workout_completions_source_check;

ALTER TABLE mentorix.client_workout_completions
  ADD CONSTRAINT client_workout_completions_source_check CHECK (source IN ('telegram', 'app'));
