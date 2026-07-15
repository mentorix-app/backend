-- sets/reps become flexible volume strings: "3", "5/4", "3-6".

ALTER TABLE mentorix.program_week_day_block_exercises
  ALTER COLUMN sets TYPE text USING sets::text,
  ALTER COLUMN reps TYPE text USING reps::text;

ALTER TABLE mentorix.program_week_day_block_exercises
  ADD CONSTRAINT program_week_day_block_exercises_sets_check CHECK (
    sets IS NULL OR sets ~ '^[0-9]+([/-][0-9]+)?$'
  ),
  ADD CONSTRAINT program_week_day_block_exercises_reps_check CHECK (
    reps IS NULL OR reps ~ '^[0-9]+([/-][0-9]+)?$'
  );

ALTER TABLE mentorix.program_version_week_day_block_exercises
  ALTER COLUMN sets TYPE text USING sets::text,
  ALTER COLUMN reps TYPE text USING reps::text;

ALTER TABLE mentorix.program_version_week_day_block_exercises
  ADD CONSTRAINT program_version_week_day_block_exercises_sets_check CHECK (
    sets IS NULL OR sets ~ '^[0-9]+([/-][0-9]+)?$'
  ),
  ADD CONSTRAINT program_version_week_day_block_exercises_reps_check CHECK (
    reps IS NULL OR reps ~ '^[0-9]+([/-][0-9]+)?$'
  );
