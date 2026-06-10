UPDATE mentorix.exercises SET muscle_group = 'full-body' WHERE muscle_group = 'full_body';
UPDATE mentorix.exercises SET type = 'mixed-modal' WHERE type = 'mixed_modal';
UPDATE mentorix.exercises SET type = 'skill-work' WHERE type = 'skill_work';
UPDATE mentorix.exercises SET equipment = REPLACE(equipment, '_', '-') WHERE equipment IS NOT NULL;

ALTER TABLE mentorix.exercises DROP CONSTRAINT exercises_muscle_group_check;
ALTER TABLE mentorix.exercises DROP CONSTRAINT exercises_type_check;
ALTER TABLE mentorix.exercises DROP CONSTRAINT exercises_equipment_check;

ALTER TABLE mentorix.exercises ADD CONSTRAINT exercises_muscle_group_check CHECK (
  muscle_group IN ('compound', 'chest', 'back', 'legs', 'shoulders', 'arms', 'core', 'full-body')
);

ALTER TABLE mentorix.exercises ADD CONSTRAINT exercises_type_check CHECK (
  type IN ('strength', 'cardio', 'mixed-modal', 'intervals', 'stretching', 'metcon', 'skill-work', 'accessory')
);

ALTER TABLE mentorix.exercises ADD CONSTRAINT exercises_equipment_check CHECK (
  equipment IS NULL OR equipment IN (
    'barbell', 'dumbbells', 'kettlebell', 'pull-up-bar', 'squat-rack', 'rowing-machine',
    'assault-bike', 'jump-rope', 'plyo-box', 'medicine-ball', 'wall-ball', 'resistance-bands',
    'battle-ropes', 'gymnastic-rings', 'sandbag', 'sled', 'weight-plates'
  )
);
