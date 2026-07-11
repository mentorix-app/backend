ALTER TABLE mentorix.program_assignments
  DROP CONSTRAINT program_assignments_status_check;

ALTER TABLE mentorix.program_assignments
  ADD CONSTRAINT program_assignments_status_check CHECK (
    status IN ('active', 'completed', 'cancelled')
  );

DROP INDEX mentorix.program_assignments_trainer_client_uniq;

CREATE UNIQUE INDEX program_assignments_trainer_client_active_uniq
  ON mentorix.program_assignments (trainer_id, client_user_id)
  WHERE status = 'active';
