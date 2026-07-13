-- Stable day identity across publish/sync + workout completion journal.

ALTER TABLE mentorix.program_week_days
  ADD COLUMN day_key uuid NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE mentorix.program_week_days
  ADD CONSTRAINT program_week_days_program_id_day_key_uniq UNIQUE (program_id, day_key);

ALTER TABLE mentorix.program_version_week_days
  ADD COLUMN day_key uuid;

UPDATE mentorix.program_version_week_days pvd
SET day_key = sub.day_key
FROM (
  SELECT
    pvd2.id AS version_day_id,
    COALESCE(pwd.day_key, gen_random_uuid()) AS day_key
  FROM mentorix.program_version_week_days pvd2
  JOIN mentorix.program_version_weeks pvw ON pvw.id = pvd2.program_version_week_id
  JOIN mentorix.program_versions pv ON pv.id = pvd2.program_version_id
  LEFT JOIN mentorix.program_weeks pw
    ON pw.program_id = pv.program_id
   AND pw.week_number = pvw.week_number
  LEFT JOIN mentorix.program_week_days pwd
    ON pwd.week_id = pw.id
   AND pwd.day_number = pvd2.day_number
) sub
WHERE pvd.id = sub.version_day_id
  AND pvd.day_key IS NULL;

UPDATE mentorix.program_version_week_days
SET day_key = gen_random_uuid()
WHERE day_key IS NULL;

ALTER TABLE mentorix.program_version_week_days
  ALTER COLUMN day_key SET NOT NULL;

ALTER TABLE mentorix.program_version_week_days
  ADD CONSTRAINT program_version_week_days_version_day_key_uniq
  UNIQUE (program_version_id, day_key);

ALTER TABLE mentorix.program_assignments
  ADD COLUMN completion_cycle_id uuid NOT NULL DEFAULT gen_random_uuid();

CREATE TABLE mentorix.client_workout_completions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  client_user_id uuid NOT NULL REFERENCES mentorix.users (id),
  trainer_id uuid NOT NULL REFERENCES mentorix.trainers (id),
  completed_at timestamptz NOT NULL,
  program_id uuid REFERENCES mentorix.programs (id) ON DELETE SET NULL,
  program_version_id uuid REFERENCES mentorix.program_versions (id) ON DELETE SET NULL,
  program_assignment_id uuid REFERENCES mentorix.program_assignments (id) ON DELETE SET NULL,
  completion_cycle_id uuid NOT NULL,
  day_key uuid NOT NULL,
  week_number int NOT NULL,
  day_number int NOT NULL,
  program_name text NOT NULL,
  program_name_ru text NOT NULL,
  day_snapshot jsonb NOT NULL,
  result_text text NOT NULL,
  source text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT client_workout_completions_source_check CHECK (source = 'telegram'),
  CONSTRAINT client_workout_completions_completion_cycle_id_day_key_uniq
    UNIQUE (completion_cycle_id, day_key)
);

CREATE INDEX client_workout_completions_client_user_id_completed_at_idx
  ON mentorix.client_workout_completions (client_user_id, completed_at DESC);

CREATE INDEX client_workout_completions_trainer_id_completed_at_idx
  ON mentorix.client_workout_completions (trainer_id, completed_at DESC);

CREATE INDEX client_workout_completions_trainer_id_client_user_id_completed_at_idx
  ON mentorix.client_workout_completions (trainer_id, client_user_id, completed_at DESC);

CREATE INDEX client_workout_completions_completion_cycle_id_idx
  ON mentorix.client_workout_completions (completion_cycle_id);
