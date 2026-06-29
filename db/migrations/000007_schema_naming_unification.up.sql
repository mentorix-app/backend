-- Unify table/column naming across mentorix schema.

-- 1. Rename tables
ALTER TABLE mentorix.refresh_sessions RENAME TO auth_refresh_sessions;
ALTER TABLE mentorix.invites RENAME TO trainer_invites;
ALTER TABLE mentorix.link_codes RENAME TO user_link_codes;

ALTER INDEX mentorix.refresh_sessions_user_id_idx RENAME TO auth_refresh_sessions_user_id_idx;
ALTER TABLE mentorix.auth_refresh_sessions
  RENAME CONSTRAINT refresh_sessions_token_hash_uniq TO auth_refresh_sessions_token_hash_uniq;

ALTER TABLE mentorix.trainer_invites
  RENAME CONSTRAINT invites_token_uniq TO trainer_invites_token_uniq;
ALTER TABLE mentorix.trainer_invites
  RENAME CONSTRAINT invites_consumed_consistency TO trainer_invites_consumed_consistency;

ALTER TABLE mentorix.user_link_codes
  RENAME CONSTRAINT link_codes_code_uniq TO user_link_codes_code_uniq;

-- 2. trainers: add surrogate id column
ALTER TABLE mentorix.trainers ADD COLUMN id uuid DEFAULT gen_random_uuid();
UPDATE mentorix.trainers SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE mentorix.trainers ALTER COLUMN id SET NOT NULL;
ALTER TABLE mentorix.trainers ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- 3. Drop FKs that reference trainers(user_id) before PK change
ALTER TABLE mentorix.trainer_clients DROP CONSTRAINT trainer_clients_trainer_user_id_fkey;
ALTER TABLE mentorix.trainer_invites DROP CONSTRAINT invites_trainer_user_id_fkey;

-- 4. Switch trainers PK to id
ALTER TABLE mentorix.trainers DROP CONSTRAINT trainers_pkey;
ALTER TABLE mentorix.trainers ADD PRIMARY KEY (id);
ALTER TABLE mentorix.trainers ADD CONSTRAINT trainers_user_id_uniq UNIQUE (user_id);

-- 5. trainer_clients: trainer_user_id -> trainer_id -> trainers(id)
ALTER TABLE mentorix.trainer_clients ADD COLUMN trainer_id uuid;
UPDATE mentorix.trainer_clients tc
SET trainer_id = t.id
FROM mentorix.trainers t
WHERE tc.trainer_user_id = t.user_id;
ALTER TABLE mentorix.trainer_clients ALTER COLUMN trainer_id SET NOT NULL;

ALTER TABLE mentorix.trainer_clients DROP CONSTRAINT trainer_clients_pkey;
ALTER TABLE mentorix.trainer_clients DROP COLUMN trainer_user_id;
ALTER TABLE mentorix.trainer_clients ADD PRIMARY KEY (trainer_id, client_user_id);
ALTER TABLE mentorix.trainer_clients ADD CONSTRAINT trainer_clients_trainer_id_fkey
  FOREIGN KEY (trainer_id) REFERENCES mentorix.trainers(id) ON DELETE CASCADE;

-- 6. trainer_invites: trainer_user_id -> trainer_id -> trainers(id)
ALTER TABLE mentorix.trainer_invites ADD COLUMN trainer_id uuid;
UPDATE mentorix.trainer_invites ti
SET trainer_id = t.id
FROM mentorix.trainers t
WHERE ti.trainer_user_id = t.user_id;
ALTER TABLE mentorix.trainer_invites ALTER COLUMN trainer_id SET NOT NULL;

ALTER TABLE mentorix.trainer_invites DROP COLUMN trainer_user_id;
ALTER TABLE mentorix.trainer_invites ADD CONSTRAINT trainer_invites_trainer_id_fkey
  FOREIGN KEY (trainer_id) REFERENCES mentorix.trainers(id) ON DELETE CASCADE;

-- 7. exercises column renames
ALTER TABLE mentorix.exercises RENAME COLUMN added_by TO created_by;
ALTER TABLE mentorix.exercises RENAME COLUMN type TO exercise_type;
ALTER TABLE mentorix.exercises RENAME CONSTRAINT exercises_type_check TO exercises_exercise_type_check;

-- 8. trainer_invites: consumed_by_user_id -> consumed_by
ALTER TABLE mentorix.trainer_invites RENAME COLUMN consumed_by_user_id TO consumed_by;

-- 9. audit columns on program children
ALTER TABLE mentorix.program_days
  ADD COLUMN modified_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN modified_by uuid NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT;

ALTER TABLE mentorix.program_day_exercises
  ADD COLUMN modified_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN modified_by uuid NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT;

-- 10. indexes
CREATE INDEX auth_identities_user_id_idx ON mentorix.auth_identities (user_id);
CREATE INDEX trainer_clients_trainer_id_idx ON mentorix.trainer_clients (trainer_id);
CREATE INDEX trainer_clients_client_user_id_idx ON mentorix.trainer_clients (client_user_id);
CREATE INDEX trainer_invites_trainer_id_idx ON mentorix.trainer_invites (trainer_id);
ALTER INDEX mentorix.program_day_exercises_day_id_idx
  RENAME TO program_day_exercises_program_day_id_idx;
