DROP INDEX IF EXISTS mentorix.program_days_week_id_idx;

ALTER TABLE mentorix.program_days
  DROP CONSTRAINT IF EXISTS program_days_week_day_number_uniq;

ALTER TABLE mentorix.program_days
  ADD CONSTRAINT program_days_program_day_number_uniq UNIQUE (program_id, day_number);

ALTER TABLE mentorix.program_days
  DROP CONSTRAINT IF EXISTS program_days_week_id_fkey;

ALTER TABLE mentorix.program_days
  DROP COLUMN IF EXISTS week_id;

DROP INDEX IF EXISTS mentorix.program_weeks_program_id_idx;

DROP TABLE IF EXISTS mentorix.program_weeks;
