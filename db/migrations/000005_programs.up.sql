CREATE TABLE mentorix.programs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  created_by uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT,
  modified_by uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'draft',
  name text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT '',
  category text NULL,
  difficulty text NULL,
  preview_image_url text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL,
  CONSTRAINT programs_status_check CHECK (status IN ('draft', 'published', 'archived')),
  CONSTRAINT programs_category_check CHECK (
    category IS NULL OR category IN (
      'weight_loss', 'muscle_gain', 'rehabilitation', 'endurance', 'functional'
    )
  ),
  CONSTRAINT programs_difficulty_check CHECK (
    difficulty IS NULL OR difficulty IN ('beginner', 'intermediate', 'advanced', 'expert')
  )
);

CREATE TABLE mentorix.program_days (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_id uuid NOT NULL REFERENCES mentorix.programs(id) ON DELETE CASCADE,
  day_number int NOT NULL,
  sort_order int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT program_days_program_day_number_uniq UNIQUE (program_id, day_number)
);

CREATE TABLE mentorix.program_day_exercises (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  program_day_id uuid NOT NULL REFERENCES mentorix.program_days(id) ON DELETE CASCADE,
  exercise_id uuid NOT NULL REFERENCES mentorix.exercises(id) ON DELETE RESTRICT,
  sort_order int NOT NULL,
  sets int NULL,
  reps int NULL,
  weight_kg numeric(6, 2) NULL,
  instruction text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX programs_created_by_idx ON mentorix.programs (created_by);
CREATE INDEX programs_status_idx ON mentorix.programs (status);
CREATE INDEX programs_deleted_at_idx ON mentorix.programs (deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX program_days_program_id_idx ON mentorix.program_days (program_id);
CREATE INDEX program_day_exercises_day_id_idx ON mentorix.program_day_exercises (program_day_id);
