DROP INDEX IF EXISTS idx_sessions_family;
ALTER TABLE sessions
    DROP COLUMN family_id,
    DROP COLUMN replaced_by,
    DROP COLUMN used_at;
