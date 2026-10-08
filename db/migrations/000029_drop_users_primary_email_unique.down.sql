CREATE UNIQUE INDEX users_primary_email_lower_uniq
  ON mentorix.users (lower(primary_email))
  WHERE primary_email IS NOT NULL;
