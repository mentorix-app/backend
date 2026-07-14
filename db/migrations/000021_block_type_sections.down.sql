-- Revert section-style block types. Fails if any row uses the new values.

ALTER TABLE mentorix.program_week_day_blocks
  DROP CONSTRAINT program_week_day_blocks_block_type_check;

ALTER TABLE mentorix.program_week_day_blocks
  ADD CONSTRAINT program_week_day_blocks_block_type_check CHECK (
    block_type IN (
      'single', 'emom', 'amrap', 'for_time', 'intervals',
      'chipper', 'ladder', 'death_by', 'superset', 'complex'
    )
  );

ALTER TABLE mentorix.program_version_week_day_blocks
  DROP CONSTRAINT program_version_week_day_blocks_block_type_check;

ALTER TABLE mentorix.program_version_week_day_blocks
  ADD CONSTRAINT program_version_week_day_blocks_block_type_check CHECK (
    block_type IN (
      'single', 'emom', 'amrap', 'for_time', 'intervals',
      'chipper', 'ladder', 'death_by', 'superset', 'complex'
    )
  );
