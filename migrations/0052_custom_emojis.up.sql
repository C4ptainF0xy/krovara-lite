CREATE TABLE custom_emojis (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id   UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    file_key   TEXT NOT NULL,
    animated   BOOLEAN NOT NULL DEFAULT FALSE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (space_id, name)
);

CREATE INDEX idx_custom_emojis_space ON custom_emojis (space_id);
