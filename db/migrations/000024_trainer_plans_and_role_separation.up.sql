-- Trainer subscription plans, exercise ownership, admin/trainer role separation.

-- 1. Exercise ownership: NULL = global admin exercise, UUID = trainer-private.
ALTER TABLE mentorix.exercises
  ADD COLUMN owner_trainer_id uuid NULL REFERENCES mentorix.trainers(id) ON DELETE RESTRICT;

CREATE INDEX exercises_owner_trainer_id_idx
  ON mentorix.exercises (owner_trainer_id)
  WHERE deleted_at IS NULL;

-- 2. Plan entitlements. Absence of an active row = Free.
CREATE TABLE mentorix.trainer_plan_entitlements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  trainer_id uuid NOT NULL REFERENCES mentorix.trainers(id) ON DELETE CASCADE,
  plan_code text NOT NULL,
  source text NOT NULL,
  status text NOT NULL DEFAULT 'active',
  valid_from timestamptz NOT NULL DEFAULT now(),
  valid_until timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT trainer_plan_entitlements_plan_code_check CHECK (plan_code IN ('free', 'advance', 'elite')),
  CONSTRAINT trainer_plan_entitlements_source_check CHECK (source IN ('admin', 'app_store', 'google_play')),
  CONSTRAINT trainer_plan_entitlements_status_check CHECK (status IN ('active', 'revoked', 'expired'))
);

CREATE INDEX trainer_plan_entitlements_trainer_id_active_idx
  ON mentorix.trainer_plan_entitlements (trainer_id)
  WHERE status = 'active';

-- One active admin grant per trainer.
CREATE UNIQUE INDEX trainer_plan_entitlements_trainer_admin_active_uniq
  ON mentorix.trainer_plan_entitlements (trainer_id)
  WHERE status = 'active' AND source = 'admin';

-- 3. Quota-count indexes.
CREATE INDEX programs_created_by_status_idx
  ON mentorix.programs (created_by, status)
  WHERE deleted_at IS NULL;

CREATE INDEX trainer_clients_trainer_id_active_idx
  ON mentorix.trainer_clients (trainer_id)
  WHERE status = 'active';

-- 4. Data migration: role separation + exercise ownership backfill.
DO $$
DECLARE
  admin_emails CONSTANT text[] := ARRAY[
    'mentorix.app@proton.me',
    'viktor.kim.developer@gmail.com',
    'zahik1311@gmail.com'
  ];
  elite_grant_email CONSTANT text := 'vicktor.ilchenko@gmail.com';
  changed_users uuid[];
  blocked_count int;
BEGIN
  -- Users whose roles will change: everyone holding admin now + listed accounts.
  SELECT COALESCE(array_agg(DISTINCT uid), '{}') INTO changed_users
  FROM (
    SELECT user_id AS uid FROM mentorix.user_roles WHERE role = 'admin'
    UNION
    SELECT id FROM mentorix.users WHERE lower(primary_email) = ANY (admin_emails)
  ) affected;

  -- 4a. Guard: listed admin accounts must not own program assignments as trainer
  -- (nothing in the audited data; fail loudly instead of losing client links silently).
  SELECT COUNT(*) INTO blocked_count
  FROM mentorix.program_assignments pa
  JOIN mentorix.trainers t ON t.id = pa.trainer_id
  JOIN mentorix.users u ON u.id = t.user_id
  WHERE lower(u.primary_email) = ANY (admin_emails);
  IF blocked_count > 0 THEN
    RAISE EXCEPTION 'migration 24: intended admin account still owns % program assignment(s); resolve manually', blocked_count;
  END IF;

  -- 4b. Hard delete trainer-owned data of listed admin accounts
  -- (per audit: zahik1311 has 1 soft-deleted draft program, 2 invites, 1 client link).
  DELETE FROM mentorix.programs p
  USING mentorix.users u
  WHERE p.created_by = u.id AND lower(u.primary_email) = ANY (admin_emails);

  DELETE FROM mentorix.trainer_invites ti
  USING mentorix.trainers t, mentorix.users u
  WHERE ti.trainer_id = t.id AND t.user_id = u.id
    AND lower(u.primary_email) = ANY (admin_emails);

  DELETE FROM mentorix.trainer_clients tc
  USING mentorix.trainers t, mentorix.users u
  WHERE tc.trainer_id = t.id AND t.user_id = u.id
    AND lower(u.primary_email) = ANY (admin_emails);

  DELETE FROM mentorix.trainers t
  USING mentorix.users u
  WHERE t.user_id = u.id AND lower(u.primary_email) = ANY (admin_emails);

  -- 4c. Listed accounts become admin-only.
  DELETE FROM mentorix.user_roles ur
  USING mentorix.users u
  WHERE ur.user_id = u.id
    AND lower(u.primary_email) = ANY (admin_emails)
    AND ur.role IN ('trainer', 'client');

  INSERT INTO mentorix.user_roles (user_id, role)
  SELECT u.id, 'admin'
  FROM mentorix.users u
  WHERE lower(u.primary_email) = ANY (admin_emails)
  ON CONFLICT (user_id, role) DO NOTHING;

  -- 4d. Every remaining admin+trainer account becomes a plain trainer.
  DELETE FROM mentorix.user_roles ur
  WHERE ur.role = 'admin'
    AND EXISTS (
      SELECT 1 FROM mentorix.user_roles tr
      WHERE tr.user_id = ur.user_id AND tr.role IN ('trainer', 'client')
    );

  -- 4e. Bind existing exercises to their creator's trainer profile.
  -- Creators without a trainers row (the listed admins after 4b) stay global.
  UPDATE mentorix.exercises e
  SET owner_trainer_id = t.id
  FROM mentorix.trainers t
  WHERE t.user_id = e.created_by
    AND e.owner_trainer_id IS NULL;

  -- 4f. Perpetual Elite grant for the trainer helping with testing.
  INSERT INTO mentorix.trainer_plan_entitlements (trainer_id, plan_code, source, status, valid_until)
  SELECT t.id, 'elite', 'admin', 'active', NULL
  FROM mentorix.trainers t
  JOIN mentorix.users u ON u.id = t.user_id
  WHERE lower(u.primary_email) = elite_grant_email;

  -- 4g. Revoke refresh sessions of every account whose roles changed.
  UPDATE mentorix.auth_refresh_sessions
  SET revoked_at = now()
  WHERE user_id = ANY (changed_users) AND revoked_at IS NULL;
END $$;

-- 5. Enforce admin exclusivity going forward.
CREATE FUNCTION mentorix.user_roles_admin_exclusive() RETURNS trigger AS $$
BEGIN
  IF NEW.role = 'admin' THEN
    IF EXISTS (
      SELECT 1 FROM mentorix.user_roles
      WHERE user_id = NEW.user_id AND role IN ('trainer', 'client')
    ) THEN
      RAISE EXCEPTION 'admin role is exclusive: user % already has trainer/client role', NEW.user_id
        USING ERRCODE = '23514';
    END IF;
  ELSE
    IF EXISTS (
      SELECT 1 FROM mentorix.user_roles
      WHERE user_id = NEW.user_id AND role = 'admin'
    ) THEN
      RAISE EXCEPTION 'admin role is exclusive: user % is an admin', NEW.user_id
        USING ERRCODE = '23514';
    END IF;
  END IF;
  RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER user_roles_admin_exclusive_trg
BEFORE INSERT OR UPDATE ON mentorix.user_roles
FOR EACH ROW EXECUTE FUNCTION mentorix.user_roles_admin_exclusive();
