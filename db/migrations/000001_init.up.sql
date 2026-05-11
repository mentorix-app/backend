-- Core schema for Mentorix (MVP foundation).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS mentorix;

CREATE TABLE IF NOT EXISTS mentorix.users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  primary_email text NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- One logical user can have multiple sign-in methods (email/password, telegram, oauth, ...).
CREATE TABLE IF NOT EXISTS mentorix.auth_identities (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE CASCADE,
  provider text NOT NULL,
  subject text NOT NULL,
  password_hash text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT auth_identities_provider_subject_uniq UNIQUE (provider, subject)
);

-- Roles are per-user and can be multiple at the same time.
CREATE TABLE IF NOT EXISTS mentorix.user_roles (
  user_id uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE CASCADE,
  role text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role),
  CONSTRAINT user_roles_role_check CHECK (role IN ('admin', 'trainer', 'client'))
);

-- A trainer is a user with the "trainer" role. Table exists for trainer-specific fields later.
CREATE TABLE IF NOT EXISTS mentorix.trainers (
  user_id uuid PRIMARY KEY REFERENCES mentorix.users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Client relationship: one client can belong to multiple trainers.
CREATE TABLE IF NOT EXISTS mentorix.trainer_clients (
  trainer_user_id uuid NOT NULL REFERENCES mentorix.trainers(user_id) ON DELETE CASCADE,
  client_user_id uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE CASCADE,
  status text NOT NULL DEFAULT 'active',
  blocked_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (trainer_user_id, client_user_id),
  CONSTRAINT trainer_clients_status_check CHECK (status IN ('active', 'blocked'))
);

-- One-time invite token (24h TTL at the application layer).
CREATE TABLE IF NOT EXISTS mentorix.invites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  trainer_user_id uuid NOT NULL REFERENCES mentorix.trainers(user_id) ON DELETE CASCADE,
  token text NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz NULL,
  consumed_by_user_id uuid NULL REFERENCES mentorix.users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT invites_token_uniq UNIQUE (token),
  CONSTRAINT invites_consumed_consistency CHECK (
    (consumed_at IS NULL AND consumed_by_user_id IS NULL) OR
    (consumed_at IS NOT NULL AND consumed_by_user_id IS NOT NULL)
  )
);

-- One-time codes used to link Telegram users to a mobile/app login later.
CREATE TABLE IF NOT EXISTS mentorix.link_codes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE CASCADE,
  code text NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT link_codes_code_uniq UNIQUE (code)
);

