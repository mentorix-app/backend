-- Revert 000004: restore CHECK constraints as in 000003 (snake_case).
-- Data stays in snake_case; do not revert to legacy kebab-case values.

ALTER TABLE mentorix.exercises DROP CONSTRAINT IF EXISTS exercises_muscle_group_check;
ALTER TABLE mentorix.exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
ALTER TABLE mentorix.exercises DROP CONSTRAINT IF EXISTS exercises_equipment_check;

ALTER TABLE mentorix.exercises ADD CONSTRAINT exercises_muscle_group_check CHECK (
  muscle_group IN ('compound', 'chest', 'back', 'legs', 'shoulders', 'arms', 'core', 'full_body')
);

ALTER TABLE mentorix.exercises ADD CONSTRAINT exercises_type_check CHECK (
  type IN ('strength', 'cardio', 'mixed_modal', 'intervals', 'stretching', 'metcon', 'skill_work', 'accessory')
);

ALTER TABLE mentorix.exercises ADD CONSTRAINT exercises_equipment_check CHECK (
  equipment IS NULL OR equipment IN (
    'barbell', 'dumbbells', 'kettlebell', 'pull_up_bar', 'squat_rack', 'rowing_machine',
    'assault_bike', 'jump_rope', 'plyo_box', 'medicine_ball', 'wall_ball', 'resistance_bands',
    'battle_ropes', 'gymnastic_rings', 'sandbag', 'sled', 'weight_plates'
  )
);
