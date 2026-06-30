CREATE TABLE mentorix.program_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id uuid NOT NULL REFERENCES mentorix.programs(id) ON DELETE CASCADE,
  version_number int NOT NULL,
  published_at timestamptz NOT NULL DEFAULT now(),
  published_by uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT,
  name text NOT NULL DEFAULT '',
  name_ru text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT '',
  description_ru text NOT NULL DEFAULT '',
  category text NULL,
  difficulty text NULL,
  preview_image_url text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT program_versions_program_version_number_uniq UNIQUE (program_id, version_number),
  CONSTRAINT program_versions_category_check CHECK (
    category IS NULL OR category IN (
      'weight_loss', 'muscle_gain', 'rehabilitation', 'endurance', 'functional'
    )
  ),
  CONSTRAINT program_versions_difficulty_check CHECK (
    difficulty IS NULL OR difficulty IN ('beginner', 'intermediate', 'advanced', 'expert')
  )
);

CREATE INDEX program_versions_program_id_idx ON mentorix.program_versions (program_id);

CREATE TABLE mentorix.program_version_weeks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_version_id uuid NOT NULL REFERENCES mentorix.program_versions(id) ON DELETE CASCADE,
  week_number int NOT NULL,
  sort_order int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT program_version_weeks_version_week_number_uniq UNIQUE (program_version_id, week_number)
);

CREATE INDEX program_version_weeks_program_version_id_idx
  ON mentorix.program_version_weeks (program_version_id);

CREATE TABLE mentorix.program_version_days (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_version_id uuid NOT NULL REFERENCES mentorix.program_versions(id) ON DELETE CASCADE,
  program_version_week_id uuid NOT NULL REFERENCES mentorix.program_version_weeks(id) ON DELETE CASCADE,
  day_number int NOT NULL,
  sort_order int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT program_version_days_week_day_number_uniq UNIQUE (program_version_week_id, day_number)
);

CREATE INDEX program_version_days_program_version_id_idx
  ON mentorix.program_version_days (program_version_id);
CREATE INDEX program_version_days_program_version_week_id_idx
  ON mentorix.program_version_days (program_version_week_id);

CREATE TABLE mentorix.program_version_day_exercises (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_version_day_id uuid NOT NULL REFERENCES mentorix.program_version_days(id) ON DELETE CASCADE,
  exercise_id uuid NOT NULL REFERENCES mentorix.exercises(id) ON DELETE RESTRICT,
  sort_order int NOT NULL,
  sets int NULL,
  reps int NULL,
  weight_kg numeric(6, 2) NULL,
  instruction text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX program_version_day_exercises_program_version_day_id_idx
  ON mentorix.program_version_day_exercises (program_version_day_id);

CREATE TABLE mentorix.program_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id uuid NOT NULL REFERENCES mentorix.programs(id) ON DELETE RESTRICT,
  program_version_id uuid NOT NULL REFERENCES mentorix.program_versions(id) ON DELETE RESTRICT,
  trainer_id uuid NOT NULL REFERENCES mentorix.trainers(id) ON DELETE RESTRICT,
  client_user_id uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'active',
  assigned_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT program_assignments_status_check CHECK (
    status IN ('active', 'completed', 'cancelled')
  )
);

CREATE UNIQUE INDEX program_assignments_trainer_client_active_uniq
  ON mentorix.program_assignments (trainer_id, client_user_id)
  WHERE status = 'active';

CREATE INDEX program_assignments_program_id_idx ON mentorix.program_assignments (program_id);
CREATE INDEX program_assignments_program_version_id_idx ON mentorix.program_assignments (program_version_id);
CREATE INDEX program_assignments_trainer_id_idx ON mentorix.program_assignments (trainer_id);
CREATE INDEX program_assignments_client_user_id_idx ON mentorix.program_assignments (client_user_id);
