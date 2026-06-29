DROP INDEX IF EXISTS idx_users_signup_ip_hash;
ALTER TABLE users DROP COLUMN IF EXISTS signup_ip_hash;
