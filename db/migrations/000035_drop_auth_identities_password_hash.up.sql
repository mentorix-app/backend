-- Fail fast instead of queueing behind a long transaction and stalling every query after it.
SET LOCAL lock_timeout = '3s';

-- Password sign-in is gone and the running code names its columns without this one,
-- so it can go while that code serves traffic.
ALTER TABLE mentorix.auth_identities
  DROP COLUMN IF EXISTS password_hash;
