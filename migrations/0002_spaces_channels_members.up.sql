CREATE TABLE spaces (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   UUID REFERENCES users(id),
    name       TEXT NOT NULL,
    icon_key   TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE channels (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id   UUID REFERENCES spaces(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    topic      TEXT,
    type       TEXT DEFAULT 'text',
    position   INT DEFAULT 0,
    is_private BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE members (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id  UUID REFERENCES spaces(id) ON DELETE CASCADE,
    user_id   UUID REFERENCES users(id) ON DELETE CASCADE,
    nickname  TEXT,
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (space_id, user_id)
);

CREATE INDEX idx_members_space_user ON members(space_id, user_id);
CREATE INDEX idx_channels_space     ON channels(space_id, position);
