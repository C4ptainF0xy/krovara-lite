CREATE TABLE banned_identifiers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind        TEXT NOT NULL CHECK (kind IN ('email', 'username')),
    value       TEXT NOT NULL,
    reason      TEXT,
    banned_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (kind, value)
);

CREATE INDEX idx_banned_identifiers_value ON banned_identifiers (kind, value);
