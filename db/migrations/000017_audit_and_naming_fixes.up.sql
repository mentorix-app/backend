-- Audit columns on program_week_day_blocks and program_assignments; align CHECK name.

ALTER TABLE mentorix.program_week_day_blocks
  ADD COLUMN modified_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN modified_by uuid NULL REFERENCES mentorix.users(id) ON DELETE SET NULL;

ALTER TABLE mentorix.program_assignments
  ADD COLUMN created_by uuid NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT,
  ADD COLUMN modified_by uuid NULL REFERENCES mentorix.users(id) ON DELETE SET NULL;

UPDATE mentorix.program_assignments pa
SET created_by = t.user_id,
    modified_by = t.user_id
FROM mentorix.trainers t
WHERE pa.trainer_id = t.id;

ALTER TABLE mentorix.program_assignments
  ALTER COLUMN created_by SET NOT NULL,
  ALTER COLUMN modified_by SET NOT NULL;

ALTER TABLE mentorix.trainer_invites
  RENAME CONSTRAINT trainer_invites_consumed_consistency TO trainer_invites_consumed_check;
