ALTER TABLE mentorix.exercises
  ADD COLUMN deleted_at timestamptz NULL;

CREATE INDEX exercises_deleted_at_idx ON mentorix.exercises (deleted_at)
  WHERE deleted_at IS NULL;
