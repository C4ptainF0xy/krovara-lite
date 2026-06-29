CREATE TABLE devices (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    ntfy_topic   TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, ntfy_topic)
);

CREATE INDEX idx_devices_user ON devices(user_id);

CREATE TABLE push_prefs (
    user_id  UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    scope    TEXT NOT NULL CHECK (scope IN ('all', 'mentions', 'none')),
    PRIMARY KEY (user_id, space_id)
);
