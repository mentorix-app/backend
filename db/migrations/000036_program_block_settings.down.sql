SET LOCAL lock_timeout = '3s';

-- Not restored by this down, the old values are gone for good:
-- - ladder, death_by and complex blocks that the up converted to skill_work, in both the
--   working and the frozen version trees;
-- - sets the up cleared in timed blocks (emom, amrap, for_time, intervals, chipper, superset);
-- - completion snapshots the up rewrote (removed block types renamed to skill_work).
-- Dropped with their values: rounds, duration_sec, time_cap_sec, work_sec and rest_sec of every
-- block in both trees.
-- A lock timeout rolls the statement back but leaves golang-migrate marking the database dirty
-- at version 35 while the schema is still at 36. Recovery: `migrate force 36`, then run the
-- down again.
ALTER TABLE mentorix.program_version_week_day_blocks
  DROP COLUMN IF EXISTS rounds,
  DROP COLUMN IF EXISTS duration_sec,
  DROP COLUMN IF EXISTS time_cap_sec,
  DROP COLUMN IF EXISTS work_sec,
  DROP COLUMN IF EXISTS rest_sec;

ALTER TABLE mentorix.program_week_day_blocks
  DROP COLUMN IF EXISTS rounds,
  DROP COLUMN IF EXISTS duration_sec,
  DROP COLUMN IF EXISTS time_cap_sec,
  DROP COLUMN IF EXISTS work_sec,
  DROP COLUMN IF EXISTS rest_sec;

ALTER TABLE mentorix.program_week_day_blocks
  DROP CONSTRAINT program_week_day_blocks_block_type_check;

ALTER TABLE mentorix.program_week_day_blocks
  ADD CONSTRAINT program_week_day_blocks_block_type_check CHECK (
    block_type IN (
      'single', 'emom', 'amrap', 'for_time', 'intervals',
      'chipper', 'ladder', 'death_by', 'superset', 'complex',
      'skill_work', 'strength', 'conditioning', 'gymnastics', 'weightlifting'
    )
  );

ALTER TABLE mentorix.program_version_week_day_blocks
  DROP CONSTRAINT program_version_week_day_blocks_block_type_check;

ALTER TABLE mentorix.program_version_week_day_blocks
  ADD CONSTRAINT program_version_week_day_blocks_block_type_check CHECK (
    block_type IN (
      'single', 'emom', 'amrap', 'for_time', 'intervals',
      'chipper', 'ladder', 'death_by', 'superset', 'complex',
      'skill_work', 'strength', 'conditioning', 'gymnastics', 'weightlifting'
    )
  );
