ALTER TABLE mentorix.trainer_invites
  RENAME CONSTRAINT trainer_invites_consumed_check TO trainer_invites_consumed_consistency;

ALTER TABLE mentorix.program_assignments
  DROP COLUMN IF EXISTS modified_by,
  DROP COLUMN IF EXISTS created_by;

ALTER TABLE mentorix.program_week_day_blocks
  DROP COLUMN IF EXISTS modified_by,
  DROP COLUMN IF EXISTS modified_at;
