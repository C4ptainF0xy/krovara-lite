DROP INDEX IF EXISTS idx_member_roles_expires;
ALTER TABLE member_roles DROP COLUMN IF EXISTS expires_at;
