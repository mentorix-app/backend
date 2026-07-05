WITH ranked_blocks AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY program_day_id
      ORDER BY sort_order ASC, created_at ASC
    ) AS new_sort
  FROM mentorix.program_day_blocks
)
UPDATE mentorix.program_day_blocks pdb
SET sort_order = ranked_blocks.new_sort
FROM ranked_blocks
WHERE pdb.id = ranked_blocks.id;

WITH ranked_exercises AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY program_day_block_id
      ORDER BY sort_order ASC, created_at ASC
    ) AS new_sort
  FROM mentorix.program_day_exercises
)
UPDATE mentorix.program_day_exercises pde
SET sort_order = ranked_exercises.new_sort
FROM ranked_exercises
WHERE pde.id = ranked_exercises.id;

WITH ranked_version_blocks AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY program_version_day_id
      ORDER BY sort_order ASC, created_at ASC
    ) AS new_sort
  FROM mentorix.program_version_day_blocks
)
UPDATE mentorix.program_version_day_blocks pvb
SET sort_order = ranked_version_blocks.new_sort
FROM ranked_version_blocks
WHERE pvb.id = ranked_version_blocks.id;

WITH ranked_version_exercises AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY program_version_day_block_id
      ORDER BY sort_order ASC, created_at ASC
    ) AS new_sort
  FROM mentorix.program_version_day_exercises
)
UPDATE mentorix.program_version_day_exercises pve
SET sort_order = ranked_version_exercises.new_sort
FROM ranked_version_exercises
WHERE pve.id = ranked_version_exercises.id;
