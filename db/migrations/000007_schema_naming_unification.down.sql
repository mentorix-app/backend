-- Reverse 000007_schema_naming_unification.

ALTER INDEX mentorix.program_day_exercises_program_day_id_idx
  RENAME TO program_day_exercises_day_id_idx;

DROP INDEX IF EXISTS mentorix.trainer_invites_trainer_id_idx;
DROP INDEX IF EXISTS mentorix.trainer_clients_client_user_id_idx;
DROP INDEX IF EXISTS mentorix.trainer_clients_trainer_id_idx;
DROP INDEX IF EXISTS mentorix.auth_identities_user_id_idx;

ALTER TABLE mentorix.program_day_exercises
  DROP COLUMN IF EXISTS modified_by,
  DROP COLUMN IF EXISTS modified_at;

ALTER TABLE mentorix.program_days
  DROP COLUMN IF EXISTS modified_by,
  DROP COLUMN IF EXISTS modified_at;

ALTER TABLE mentorix.trainer_invites RENAME COLUMN consumed_by TO consumed_by_user_id;

ALTER TABLE mentorix.exercises RENAME CONSTRAINT exercises_exercise_type_check TO exercises_type_check;
ALTER TABLE mentorix.exercises RENAME COLUMN exercise_type TO type;
ALTER TABLE mentorix.exercises RENAME COLUMN created_by TO added_by;

ALTER TABLE mentorix.trainer_invites DROP CONSTRAINT trainer_invites_trainer_id_fkey;
ALTER TABLE mentorix.trainer_invites ADD COLUMN trainer_user_id uuid;
UPDATE mentorix.trainer_invites ti
SET trainer_user_id = t.user_id
FROM mentorix.trainers t
WHERE ti.trainer_id = t.id;
ALTER TABLE mentorix.trainer_invites ALTER COLUMN trainer_user_id SET NOT NULL;
ALTER TABLE mentorix.trainer_invites DROP COLUMN trainer_id;
ALTER TABLE mentorix.trainer_invites ADD CONSTRAINT invites_trainer_user_id_fkey
  FOREIGN KEY (trainer_user_id) REFERENCES mentorix.trainers(user_id) ON DELETE CASCADE;

ALTER TABLE mentorix.trainer_clients DROP CONSTRAINT trainer_clients_trainer_id_fkey;
ALTER TABLE mentorix.trainer_clients DROP CONSTRAINT trainer_clients_pkey;
ALTER TABLE mentorix.trainer_clients ADD COLUMN trainer_user_id uuid;
UPDATE mentorix.trainer_clients tc
SET trainer_user_id = t.user_id
FROM mentorix.trainers t
WHERE tc.trainer_id = t.id;
ALTER TABLE mentorix.trainer_clients ALTER COLUMN trainer_user_id SET NOT NULL;
ALTER TABLE mentorix.trainer_clients DROP COLUMN trainer_id;
ALTER TABLE mentorix.trainer_clients ADD PRIMARY KEY (trainer_user_id, client_user_id);
ALTER TABLE mentorix.trainer_clients ADD CONSTRAINT trainer_clients_trainer_user_id_fkey
  FOREIGN KEY (trainer_user_id) REFERENCES mentorix.trainers(user_id) ON DELETE CASCADE;

ALTER TABLE mentorix.trainers DROP CONSTRAINT trainers_user_id_uniq;
ALTER TABLE mentorix.trainers DROP CONSTRAINT trainers_pkey;
ALTER TABLE mentorix.trainers ADD PRIMARY KEY (user_id);
ALTER TABLE mentorix.trainers DROP COLUMN id;

ALTER TABLE mentorix.user_link_codes
  RENAME CONSTRAINT user_link_codes_code_uniq TO link_codes_code_uniq;
ALTER TABLE mentorix.trainer_invites
  RENAME CONSTRAINT trainer_invites_consumed_consistency TO invites_consumed_consistency;
ALTER TABLE mentorix.trainer_invites
  RENAME CONSTRAINT trainer_invites_token_uniq TO invites_token_uniq;
ALTER TABLE mentorix.auth_refresh_sessions
  RENAME CONSTRAINT auth_refresh_sessions_token_hash_uniq TO refresh_sessions_token_hash_uniq;
ALTER INDEX mentorix.auth_refresh_sessions_user_id_idx RENAME TO refresh_sessions_user_id_idx;

ALTER TABLE mentorix.user_link_codes RENAME TO link_codes;
ALTER TABLE mentorix.trainer_invites RENAME TO invites;
ALTER TABLE mentorix.auth_refresh_sessions RENAME TO refresh_sessions;
