ALTER TABLE mentorix.exercises
  ALTER COLUMN name DROP DEFAULT;

ALTER TABLE mentorix.programs
  ALTER COLUMN name DROP NOT NULL,
  ALTER COLUMN name DROP DEFAULT;

UPDATE mentorix.programs SET name = NULL WHERE name = '';
