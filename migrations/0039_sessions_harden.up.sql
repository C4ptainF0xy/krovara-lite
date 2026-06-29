ALTER TABLE sessions
    ADD COLUMN family_id   UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN replaced_by UUID,
    ADD COLUMN used_at     TIMESTAMPTZ;

CREATE INDEX idx_sessions_family ON sessions(family_id);
