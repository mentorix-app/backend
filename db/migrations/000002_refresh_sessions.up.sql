CREATE TABLE mentorix.refresh_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT refresh_sessions_token_hash_uniq UNIQUE (token_hash)
);

CREATE INDEX refresh_sessions_user_id_idx ON mentorix.refresh_sessions (user_id);
