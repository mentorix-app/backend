DROP INDEX IF EXISTS mentorix.auth_refresh_sessions_family_id_live_uniq;

ALTER TABLE mentorix.auth_refresh_sessions
  DROP COLUMN IF EXISTS rotated_at,
  DROP COLUMN IF EXISTS family_id;
