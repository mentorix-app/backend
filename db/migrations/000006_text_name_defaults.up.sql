UPDATE mentorix.programs SET name = '' WHERE name IS NULL;

ALTER TABLE mentorix.programs
  ALTER COLUMN name SET DEFAULT '',
  ALTER COLUMN name SET NOT NULL;

ALTER TABLE mentorix.exercises
  ALTER COLUMN name SET DEFAULT '';
