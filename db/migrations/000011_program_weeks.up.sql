CREATE TABLE mentorix.program_weeks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id uuid NOT NULL REFERENCES mentorix.programs(id) ON DELETE CASCADE,
  week_number int NOT NULL,
  sort_order int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now(),
  modified_by uuid NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT,
  CONSTRAINT program_weeks_program_week_number_uniq UNIQUE (program_id, week_number)
);

CREATE INDEX program_weeks_program_id_idx ON mentorix.program_weeks (program_id);

ALTER TABLE mentorix.program_days ADD COLUMN week_id uuid NULL;

INSERT INTO mentorix.program_weeks (program_id, week_number, sort_order)
SELECT DISTINCT program_id, 1, 1
FROM mentorix.program_days;

INSERT INTO mentorix.program_weeks (program_id, week_number, sort_order)
SELECT p.id, 1, 1
FROM mentorix.programs p
WHERE p.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM mentorix.program_weeks pw WHERE pw.program_id = p.id
  );

UPDATE mentorix.program_days pd
SET week_id = pw.id
FROM mentorix.program_weeks pw
WHERE pw.program_id = pd.program_id
  AND pw.week_number = 1
  AND pd.week_id IS NULL;

ALTER TABLE mentorix.program_days
  ALTER COLUMN week_id SET NOT NULL;

ALTER TABLE mentorix.program_days
  ADD CONSTRAINT program_days_week_id_fkey
  FOREIGN KEY (week_id) REFERENCES mentorix.program_weeks(id) ON DELETE CASCADE;

ALTER TABLE mentorix.program_days
  DROP CONSTRAINT program_days_program_day_number_uniq;

ALTER TABLE mentorix.program_days
  ADD CONSTRAINT program_days_week_day_number_uniq UNIQUE (week_id, day_number);

CREATE INDEX program_days_week_id_idx ON mentorix.program_days (week_id);
