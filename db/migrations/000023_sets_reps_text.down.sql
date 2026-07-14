-- Revert sets/reps to int. Non-integer strings become NULL.

ALTER TABLE mentorix.program_week_day_block_exercises
  DROP CONSTRAINT IF EXISTS program_week_day_block_exercises_sets_check,
  DROP CONSTRAINT IF EXISTS program_week_day_block_exercises_reps_check;

ALTER TABLE mentorix.program_week_day_block_exercises
  ALTER COLUMN sets TYPE int USING (
    CASE WHEN sets ~ '^[0-9]+$' THEN sets::integer ELSE NULL END
  ),
  ALTER COLUMN reps TYPE int USING (
    CASE WHEN reps ~ '^[0-9]+$' THEN reps::integer ELSE NULL END
  );

ALTER TABLE mentorix.program_version_week_day_block_exercises
  DROP CONSTRAINT IF EXISTS program_version_week_day_block_exercises_sets_check,
  DROP CONSTRAINT IF EXISTS program_version_week_day_block_exercises_reps_check;

ALTER TABLE mentorix.program_version_week_day_block_exercises
  ALTER COLUMN sets TYPE int USING (
    CASE WHEN sets ~ '^[0-9]+$' THEN sets::integer ELSE NULL END
  ),
  ALTER COLUMN reps TYPE int USING (
    CASE WHEN reps ~ '^[0-9]+$' THEN reps::integer ELSE NULL END
  );
