-- The application code that relied on this index was reverted. Registration enforces
-- unique emails through auth_identities (provider, subject), as it did before 000027.
DROP INDEX IF EXISTS mentorix.users_primary_email_lower_uniq;
