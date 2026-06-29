CREATE TABLE report_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id  UUID NOT NULL REFERENCES reports(id) ON DELETE CASCADE,
    author_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_report_comments_report ON report_comments (report_id, created_at);
