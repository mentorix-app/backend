-- Fail fast instead of queueing behind a long transaction and stalling every query after it.
SET LOCAL lock_timeout = '3s';

-- "Become a trainer" records the version of the trainer terms the person accepted and when.
-- Trainers from email registration have no acceptance on record, so both columns may be null,
-- but never only one of them.
ALTER TABLE mentorix.trainers
  ADD COLUMN terms_version text,
  ADD COLUMN terms_accepted_at timestamptz,
  ADD CONSTRAINT trainers_terms_check CHECK ((terms_version IS NULL) = (terms_accepted_at IS NULL));
