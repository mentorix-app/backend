-- family_id groups the sessions descended from one sign-in; the default gives each
-- existing row and each new sign-in its own family. rotated_at marks a token spent by rotation.
ALTER TABLE mentorix.auth_refresh_sessions
  ADD COLUMN family_id uuid NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN rotated_at timestamptz NULL;

-- A family has at most one unrevoked session. Rotation spends the live token before it
-- inserts the successor, so the index holds per statement. It also serves the family queries.
CREATE UNIQUE INDEX auth_refresh_sessions_family_id_live_uniq
  ON mentorix.auth_refresh_sessions (family_id)
  WHERE revoked_at IS NULL;
