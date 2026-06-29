DROP INDEX IF EXISTS idx_files_scan_status;
ALTER TABLE files DROP COLUMN IF EXISTS scan_status;
