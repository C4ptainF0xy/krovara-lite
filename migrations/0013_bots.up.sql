CREATE TABLE bots (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id      UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    component_jid TEXT NOT NULL UNIQUE,
    secret_hash   TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bots_space ON bots(space_id);
