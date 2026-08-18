DROP TABLE IF EXISTS mentorix.program_block_clients;

DROP INDEX IF EXISTS mentorix.program_version_week_day_blocks_block_key_idx;
ALTER TABLE mentorix.program_version_week_day_blocks
  DROP COLUMN IF EXISTS block_key;

DROP INDEX IF EXISTS mentorix.program_week_day_blocks_block_key_idx;
ALTER TABLE mentorix.program_week_day_blocks
  DROP COLUMN IF EXISTS block_key;
