-- Fail fast instead of queueing behind a long transaction and stalling every query after it.
-- If this migration fails, the whole transaction rolls back and golang-migrate marks version 36
-- dirty. Recovery: `migrate force 35`, fix the cause, redeploy.
SET LOCAL lock_timeout = '3s';

-- Block writes of the old code wait for the few milliseconds of this migration, so none of them
-- can commit a removed block type between the UPDATE below and the new CHECK.
LOCK TABLE
  mentorix.program_week_day_blocks,
  mentorix.program_version_week_day_blocks,
  mentorix.program_week_day_block_exercises,
  mentorix.program_version_week_day_block_exercises
IN SHARE ROW EXCLUSIVE MODE;

-- The old code keeps serving for about a minute after this migration. Every query names its
-- columns, so the new ones do not affect it. Two effects in that window:
-- 1. A removed block type written by old code (merge defaults to complex, PATCH to complex,
--    ladder or death_by) fails the new CHECK. That is a short, retryable error and no data is
--    lost.
-- 2. Old code can still save sets into a block whose type has none (emom, amrap, for_time,
--    intervals, chipper, superset). Those sets stay visible in the API, the bot and snapshots
--    until the next PATCH of that block clears them, and a PUT of that exercise with the same
--    sets gets 400.

-- Ladder, death-by and complex blocks become skill work.
UPDATE mentorix.program_week_day_blocks
SET block_type = 'skill_work'
WHERE block_type IN ('ladder', 'death_by', 'complex');

UPDATE mentorix.program_version_week_day_blocks
SET block_type = 'skill_work'
WHERE block_type IN ('ladder', 'death_by', 'complex');

-- Timed formats carry no sets from now on. Reps and timestamps stay as they are.
UPDATE mentorix.program_week_day_block_exercises e
SET sets = NULL
FROM mentorix.program_week_day_blocks b
WHERE b.id = e.program_week_day_block_id
  AND b.block_type IN ('emom', 'amrap', 'for_time', 'intervals', 'chipper', 'superset')
  AND e.sets IS NOT NULL;

UPDATE mentorix.program_version_week_day_block_exercises e
SET sets = NULL
FROM mentorix.program_version_week_day_blocks b
WHERE b.id = e.program_version_week_day_block_id
  AND b.block_type IN ('emom', 'amrap', 'for_time', 'intervals', 'chipper', 'superset')
  AND e.sets IS NOT NULL;

-- Completed days keep their snapshot; only the block type of the removed formats is renamed.
-- Element order and every other key stay, sets included.
UPDATE mentorix.client_workout_completions
SET day_snapshot = jsonb_set(
  day_snapshot,
  '{blocks}',
  (
    SELECT jsonb_agg(
      CASE
        WHEN t.block ->> 'block_type' IN ('ladder', 'death_by', 'complex')
          THEN jsonb_set(t.block, '{block_type}', '"skill_work"')
        ELSE t.block
      END
      ORDER BY t.position
    )
    FROM jsonb_array_elements(day_snapshot -> 'blocks') WITH ORDINALITY AS t(block, position)
  )
)
-- Only snapshots whose blocks is an array: jsonb_array_elements fails on an object, and the
-- path below would match an object holding a removed type.
WHERE jsonb_typeof(day_snapshot -> 'blocks') = 'array'
  AND jsonb_path_exists(
    day_snapshot,
    '$.blocks[*] ? (@.block_type == "ladder" || @.block_type == "death_by" || @.block_type == "complex")'
  );

ALTER TABLE mentorix.program_week_day_blocks
  DROP CONSTRAINT program_week_day_blocks_block_type_check;

ALTER TABLE mentorix.program_week_day_blocks
  ADD CONSTRAINT program_week_day_blocks_block_type_check CHECK (
    block_type IN (
      'single', 'emom', 'amrap', 'for_time', 'intervals',
      'chipper', 'superset',
      'skill_work', 'strength', 'conditioning', 'gymnastics', 'weightlifting'
    )
  );

ALTER TABLE mentorix.program_version_week_day_blocks
  DROP CONSTRAINT program_version_week_day_blocks_block_type_check;

ALTER TABLE mentorix.program_version_week_day_blocks
  ADD CONSTRAINT program_version_week_day_blocks_block_type_check CHECK (
    block_type IN (
      'single', 'emom', 'amrap', 'for_time', 'intervals',
      'chipper', 'superset',
      'skill_work', 'strength', 'conditioning', 'gymnastics', 'weightlifting'
    )
  );

-- Format settings of a group block. All stay NULL until a trainer sets them.
ALTER TABLE mentorix.program_week_day_blocks
  ADD COLUMN IF NOT EXISTS rounds int NULL,
  ADD COLUMN IF NOT EXISTS duration_sec int NULL,
  ADD COLUMN IF NOT EXISTS time_cap_sec int NULL,
  ADD COLUMN IF NOT EXISTS work_sec int NULL,
  ADD COLUMN IF NOT EXISTS rest_sec int NULL;

ALTER TABLE mentorix.program_week_day_blocks
  ADD CONSTRAINT program_week_day_blocks_rounds_check CHECK (rounds BETWEEN 1 AND 100),
  ADD CONSTRAINT program_week_day_blocks_duration_sec_check CHECK (duration_sec BETWEEN 1 AND 86400),
  ADD CONSTRAINT program_week_day_blocks_time_cap_sec_check CHECK (time_cap_sec BETWEEN 1 AND 86400),
  ADD CONSTRAINT program_week_day_blocks_work_sec_check CHECK (work_sec BETWEEN 1 AND 86400),
  ADD CONSTRAINT program_week_day_blocks_rest_sec_check CHECK (rest_sec BETWEEN 0 AND 86400);

ALTER TABLE mentorix.program_version_week_day_blocks
  ADD COLUMN IF NOT EXISTS rounds int NULL,
  ADD COLUMN IF NOT EXISTS duration_sec int NULL,
  ADD COLUMN IF NOT EXISTS time_cap_sec int NULL,
  ADD COLUMN IF NOT EXISTS work_sec int NULL,
  ADD COLUMN IF NOT EXISTS rest_sec int NULL;

ALTER TABLE mentorix.program_version_week_day_blocks
  ADD CONSTRAINT program_version_week_day_blocks_rounds_check CHECK (rounds BETWEEN 1 AND 100),
  ADD CONSTRAINT program_version_week_day_blocks_duration_sec_check CHECK (duration_sec BETWEEN 1 AND 86400),
  ADD CONSTRAINT program_version_week_day_blocks_time_cap_sec_check CHECK (time_cap_sec BETWEEN 1 AND 86400),
  ADD CONSTRAINT program_version_week_day_blocks_work_sec_check CHECK (work_sec BETWEEN 1 AND 86400),
  ADD CONSTRAINT program_version_week_day_blocks_rest_sec_check CHECK (rest_sec BETWEEN 0 AND 86400);
