CREATE TABLE mentorix.exercises (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL DEFAULT '',
  name_ru text NOT NULL DEFAULT '',
  added_by uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT,
  modified_by uuid NOT NULL REFERENCES mentorix.users(id) ON DELETE RESTRICT,
  modified_at timestamptz NOT NULL DEFAULT now(),
  equipment text NULL,
  type text NOT NULL,
  muscle_group text NOT NULL,
  description text NOT NULL DEFAULT '',
  description_ru text NOT NULL DEFAULT '',
  difficulty text NOT NULL,
  video_url text NOT NULL DEFAULT '',
  preview_image_url text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT exercises_muscle_group_check CHECK (
    muscle_group IN ('compound', 'chest', 'back', 'legs', 'shoulders', 'arms', 'core', 'full_body')
  ),
  CONSTRAINT exercises_type_check CHECK (
    type IN ('strength', 'cardio', 'mixed_modal', 'intervals', 'stretching', 'metcon', 'skill_work', 'accessory')
  ),
  CONSTRAINT exercises_equipment_check CHECK (
    equipment IS NULL OR equipment IN (
      'barbell', 'dumbbells', 'kettlebell', 'pull_up_bar', 'squat_rack', 'rowing_machine',
      'assault_bike', 'jump_rope', 'plyo_box', 'medicine_ball', 'wall_ball', 'resistance_bands',
      'battle_ropes', 'gymnastic_rings', 'sandbag', 'sled', 'weight_plates'
    )
  ),
  CONSTRAINT exercises_difficulty_check CHECK (
    difficulty IN ('beginner', 'intermediate', 'advanced', 'expert')
  )
);

CREATE INDEX exercises_name_idx ON mentorix.exercises (name);
