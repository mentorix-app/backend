CREATE TABLE mentorix.program_day_blocks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_day_id uuid NOT NULL REFERENCES mentorix.program_days(id) ON DELETE CASCADE,
  block_type text NOT NULL DEFAULT 'single',
  instruction text NOT NULL DEFAULT '',
  sort_order int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT program_day_blocks_block_type_check CHECK (
    block_type IN (
      'single', 'emom', 'amrap', 'for_time', 'intervals',
      'chipper', 'ladder', 'death_by', 'superset', 'complex'
    )
  )
);

CREATE INDEX program_day_blocks_program_day_id_idx
  ON mentorix.program_day_blocks (program_day_id);

ALTER TABLE mentorix.program_day_exercises
  ADD COLUMN program_day_block_id uuid NULL;

DO $$
DECLARE
  r RECORD;
  new_block_id uuid;
BEGIN
  FOR r IN
    SELECT id, program_day_id, sort_order, created_at
    FROM mentorix.program_day_exercises
    ORDER BY program_day_id, sort_order, created_at
  LOOP
    INSERT INTO mentorix.program_day_blocks (
      program_day_id, block_type, instruction, sort_order, created_at
    ) VALUES (
      r.program_day_id, 'single', '', r.sort_order, r.created_at
    ) RETURNING id INTO new_block_id;

    UPDATE mentorix.program_day_exercises
    SET program_day_block_id = new_block_id
    WHERE id = r.id;
  END LOOP;
END $$;

ALTER TABLE mentorix.program_day_exercises
  ALTER COLUMN program_day_block_id SET NOT NULL;

ALTER TABLE mentorix.program_day_exercises
  ADD CONSTRAINT program_day_exercises_program_day_block_id_fkey
  FOREIGN KEY (program_day_block_id) REFERENCES mentorix.program_day_blocks(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS mentorix.program_day_exercises_program_day_id_idx;

ALTER TABLE mentorix.program_day_exercises
  DROP COLUMN program_day_id;

ALTER TABLE mentorix.program_day_exercises
  DROP COLUMN weight_kg;

CREATE INDEX program_day_exercises_program_day_block_id_idx
  ON mentorix.program_day_exercises (program_day_block_id);

CREATE TABLE mentorix.program_version_day_blocks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_version_day_id uuid NOT NULL REFERENCES mentorix.program_version_days(id) ON DELETE CASCADE,
  block_type text NOT NULL DEFAULT 'single',
  instruction text NOT NULL DEFAULT '',
  sort_order int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT program_version_day_blocks_block_type_check CHECK (
    block_type IN (
      'single', 'emom', 'amrap', 'for_time', 'intervals',
      'chipper', 'ladder', 'death_by', 'superset', 'complex'
    )
  )
);

CREATE INDEX program_version_day_blocks_program_version_day_id_idx
  ON mentorix.program_version_day_blocks (program_version_day_id);

ALTER TABLE mentorix.program_version_day_exercises
  ADD COLUMN program_version_day_block_id uuid NULL;

DO $$
DECLARE
  r RECORD;
  new_block_id uuid;
BEGIN
  FOR r IN
    SELECT id, program_version_day_id, sort_order, created_at
    FROM mentorix.program_version_day_exercises
    ORDER BY program_version_day_id, sort_order, created_at
  LOOP
    INSERT INTO mentorix.program_version_day_blocks (
      program_version_day_id, block_type, instruction, sort_order, created_at
    ) VALUES (
      r.program_version_day_id, 'single', '', r.sort_order, r.created_at
    ) RETURNING id INTO new_block_id;

    UPDATE mentorix.program_version_day_exercises
    SET program_version_day_block_id = new_block_id
    WHERE id = r.id;
  END LOOP;
END $$;

ALTER TABLE mentorix.program_version_day_exercises
  ALTER COLUMN program_version_day_block_id SET NOT NULL;

ALTER TABLE mentorix.program_version_day_exercises
  ADD CONSTRAINT program_version_day_exercises_program_version_day_block_id_fkey
  FOREIGN KEY (program_version_day_block_id) REFERENCES mentorix.program_version_day_blocks(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS mentorix.program_version_day_exercises_program_version_day_id_idx;

ALTER TABLE mentorix.program_version_day_exercises
  DROP COLUMN program_version_day_id;

ALTER TABLE mentorix.program_version_day_exercises
  DROP COLUMN weight_kg;

CREATE INDEX program_version_day_exercises_program_version_day_block_id_idx
  ON mentorix.program_version_day_exercises (program_version_day_block_id);
