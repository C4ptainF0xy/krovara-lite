ALTER TABLE reports
    ADD COLUMN space_id    UUID REFERENCES spaces(id) ON DELETE CASCADE,
    ADD COLUMN channel_id  UUID REFERENCES channels(id) ON DELETE SET NULL,
    ADD COLUMN resolved_by UUID REFERENCES users(id),
    ADD COLUMN resolved_at TIMESTAMPTZ;

CREATE INDEX idx_reports_space_status ON reports(space_id, status);
CREATE INDEX idx_reports_status        ON reports(status);
