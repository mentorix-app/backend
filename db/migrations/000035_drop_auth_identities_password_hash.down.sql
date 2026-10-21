SET LOCAL lock_timeout = '3s';

-- The column comes back empty: the dropped hashes are gone for good. A lock timeout rolls the
-- statement back but leaves golang-migrate marking the database dirty at version 34 while the
-- schema is still at 35. Recovery: `migrate force 35`, then run the down again.
ALTER TABLE mentorix.auth_identities
  ADD COLUMN IF NOT EXISTS password_hash text NULL;
