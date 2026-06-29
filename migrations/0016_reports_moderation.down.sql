DROP INDEX IF EXISTS idx_reports_status;
DROP INDEX IF EXISTS idx_reports_space_status;
ALTER TABLE reports
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS resolved_by,
    DROP COLUMN IF EXISTS channel_id,
    DROP COLUMN IF EXISTS space_id;
