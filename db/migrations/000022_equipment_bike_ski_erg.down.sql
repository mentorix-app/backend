-- Revert bike_erg / ski_erg. Fails if any row uses the new values.

ALTER TABLE mentorix.exercises DROP CONSTRAINT exercises_equipment_check;

ALTER TABLE mentorix.exercises ADD CONSTRAINT exercises_equipment_check CHECK (
  equipment IS NULL OR equipment IN (
    'barbell', 'dumbbells', 'kettlebell', 'pull_up_bar', 'squat_rack', 'rowing_machine',
    'assault_bike', 'jump_rope', 'plyo_box', 'medicine_ball', 'wall_ball', 'resistance_bands',
    'battle_ropes', 'gymnastic_rings', 'sandbag', 'sled', 'weight_plates'
  )
);
