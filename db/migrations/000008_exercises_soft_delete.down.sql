DROP INDEX IF EXISTS mentorix.exercises_deleted_at_idx;

ALTER TABLE mentorix.exercises
  DROP COLUMN IF EXISTS deleted_at;
