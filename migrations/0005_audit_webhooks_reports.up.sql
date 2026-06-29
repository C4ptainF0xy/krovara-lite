CREATE TABLE audit_logs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id   UUID REFERENCES spaces(id) ON DELETE CASCADE,
    actor_id   UUID REFERENCES users(id),
    action     TEXT NOT NULL,
    target_id  UUID,
    metadata   JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE webhooks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id   UUID REFERENCES spaces(id) ON DELETE CASCADE,
    channel_id UUID REFERENCES channels(id),
    name       TEXT NOT NULL,
    url        TEXT NOT NULL,
    secret     TEXT,
    events     TEXT[] NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE reports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id UUID REFERENCES users(id),
    target_type TEXT NOT NULL,
    target_id   UUID NOT NULL,
    reason      TEXT NOT NULL,
    status      TEXT DEFAULT 'pending',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_space_created ON audit_logs(space_id, created_at DESC);
CREATE INDEX idx_webhooks_space      ON webhooks(space_id);
