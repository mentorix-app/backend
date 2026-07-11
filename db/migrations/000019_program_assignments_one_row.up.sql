-- One assignment row per (trainer_id, client_user_id); clear deletes the row.

DELETE FROM mentorix.program_assignments
WHERE status = 'cancelled';

DROP INDEX mentorix.program_assignments_trainer_client_active_uniq;

CREATE UNIQUE INDEX program_assignments_trainer_client_uniq
  ON mentorix.program_assignments (trainer_id, client_user_id);

ALTER TABLE mentorix.program_assignments
  DROP CONSTRAINT program_assignments_status_check;

ALTER TABLE mentorix.program_assignments
  ADD CONSTRAINT program_assignments_status_check CHECK (status = 'active');
