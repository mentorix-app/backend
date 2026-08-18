-- Stable block identity across publish/discard + per-client block visibility rules.

ALTER TABLE mentorix.program_week_day_blocks
  ADD COLUMN block_key uuid NOT NULL DEFAULT gen_random_uuid();

CREATE INDEX program_week_day_blocks_block_key_idx
  ON mentorix.program_week_day_blocks (block_key);

ALTER TABLE mentorix.program_version_week_day_blocks
  ADD COLUMN block_key uuid NOT NULL DEFAULT gen_random_uuid();

-- Best-effort backfill: point a frozen block at its template block's key, matched
-- by (week_number, day_number, sort_order). Same approach 000020 used for day_key.
-- Rows with no template match keep the random key the column default gave them.
UPDATE mentorix.program_version_week_day_blocks vb
SET block_key = sub.block_key
FROM (
  SELECT
    vb2.id AS version_block_id,
    tb.block_key AS block_key
  FROM mentorix.program_version_week_day_blocks vb2
  JOIN mentorix.program_version_week_days vd
    ON vd.id = vb2.program_version_week_day_id
  JOIN mentorix.program_version_weeks vw
    ON vw.id = vd.program_version_week_id
  JOIN mentorix.program_versions v
    ON v.id = vd.program_version_id
  JOIN mentorix.program_weeks tw
    ON tw.program_id = v.program_id
   AND tw.week_number = vw.week_number
  JOIN mentorix.program_week_days td
    ON td.week_id = tw.id
   AND td.day_number = vd.day_number
  JOIN mentorix.program_week_day_blocks tb
    ON tb.program_week_day_id = td.id
   AND tb.sort_order = vb2.sort_order
) sub
WHERE vb.id = sub.version_block_id;

CREATE INDEX program_version_week_day_blocks_block_key_idx
  ON mentorix.program_version_week_day_blocks (block_key);

CREATE TABLE mentorix.program_block_clients (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id     uuid NOT NULL REFERENCES mentorix.programs (id) ON DELETE CASCADE,
  block_key      uuid NOT NULL,
  client_user_id uuid NOT NULL REFERENCES mentorix.users (id) ON DELETE CASCADE,
  created_at     timestamptz NOT NULL DEFAULT now(),
  created_by     uuid REFERENCES mentorix.users (id),
  CONSTRAINT program_block_clients_program_block_key_client_uniq
    UNIQUE (program_id, block_key, client_user_id)
);

CREATE INDEX program_block_clients_program_id_block_key_idx
  ON mentorix.program_block_clients (program_id, block_key);

CREATE INDEX program_block_clients_client_user_id_idx
  ON mentorix.program_block_clients (client_user_id);
