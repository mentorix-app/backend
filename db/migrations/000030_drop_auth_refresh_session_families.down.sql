ALTER TABLE mentorix.auth_refresh_sessions
  ADD COLUMN family_id uuid NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN rotated_at timestamptz NULL;

CREATE UNIQUE INDEX auth_refresh_sessions_family_id_live_uniq
  ON mentorix.auth_refresh_sessions (family_id)
  WHERE revoked_at IS NULL;
