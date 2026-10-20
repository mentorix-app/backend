SET LOCAL lock_timeout = '3s';

-- Drops the recorded terms acceptance for good. No data can stop it. A lock timeout rolls the
-- statements back but leaves golang-migrate marking the database dirty at version 33 while the
-- schema is still at 34. Recovery: `migrate force 34`, then run the down again.
ALTER TABLE mentorix.trainers
  DROP CONSTRAINT trainers_terms_check,
  DROP COLUMN terms_accepted_at,
  DROP COLUMN terms_version;
