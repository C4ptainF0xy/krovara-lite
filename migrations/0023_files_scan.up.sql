ALTER TABLE files
    ADD COLUMN scan_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (scan_status IN ('pending', 'clean', 'infected', 'error'));

CREATE INDEX idx_files_scan_status ON files (scan_status);
