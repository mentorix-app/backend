DROP INDEX IF EXISTS mentorix.program_version_day_exercises_program_version_day_block_id_idx;

ALTER TABLE mentorix.program_version_day_exercises
  ADD COLUMN program_version_day_id uuid NULL;

ALTER TABLE mentorix.program_version_day_exercises
  ADD COLUMN weight_kg numeric(6, 2) NULL;

UPDATE mentorix.program_version_day_exercises pde
SET program_version_day_id = pvb.program_version_day_id
FROM mentorix.program_version_day_blocks pvb
WHERE pde.program_version_day_block_id = pvb.id;

ALTER TABLE mentorix.program_version_day_exercises
  ALTER COLUMN program_version_day_id SET NOT NULL;

ALTER TABLE mentorix.program_version_day_exercises
  DROP CONSTRAINT IF EXISTS program_version_day_exercises_program_version_day_block_id_fkey;

ALTER TABLE mentorix.program_version_day_exercises
  DROP COLUMN program_version_day_block_id;

ALTER TABLE mentorix.program_version_day_exercises
  ADD CONSTRAINT program_version_day_exercises_program_version_day_id_fkey
  FOREIGN KEY (program_version_day_id) REFERENCES mentorix.program_version_days(id) ON DELETE CASCADE;

CREATE INDEX program_version_day_exercises_program_version_day_id_idx
  ON mentorix.program_version_day_exercises (program_version_day_id);

DROP TABLE IF EXISTS mentorix.program_version_day_blocks;

DROP INDEX IF EXISTS mentorix.program_day_exercises_program_day_block_id_idx;

ALTER TABLE mentorix.program_day_exercises
  ADD COLUMN program_day_id uuid NULL;

ALTER TABLE mentorix.program_day_exercises
  ADD COLUMN weight_kg numeric(6, 2) NULL;

UPDATE mentorix.program_day_exercises pde
SET program_day_id = pdb.program_day_id
FROM mentorix.program_day_blocks pdb
WHERE pde.program_day_block_id = pdb.id;

ALTER TABLE mentorix.program_day_exercises
  ALTER COLUMN program_day_id SET NOT NULL;

ALTER TABLE mentorix.program_day_exercises
  DROP CONSTRAINT IF EXISTS program_day_exercises_program_day_block_id_fkey;

ALTER TABLE mentorix.program_day_exercises
  DROP COLUMN program_day_block_id;

ALTER TABLE mentorix.program_day_exercises
  ADD CONSTRAINT program_day_exercises_program_day_id_fkey
  FOREIGN KEY (program_day_id) REFERENCES mentorix.program_days(id) ON DELETE CASCADE;

CREATE INDEX program_day_exercises_program_day_id_idx
  ON mentorix.program_day_exercises (program_day_id);

DROP TABLE IF EXISTS mentorix.program_day_blocks;
