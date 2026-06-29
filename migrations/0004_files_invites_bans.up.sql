CREATE TABLE files (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   UUID REFERENCES users(id),
    filename   TEXT NOT NULL,
    size       BIGINT NOT NULL,
    mimetype   TEXT NOT NULL,
    path       TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE invites (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id   UUID REFERENCES spaces(id) ON DELETE CASCADE,
    creator_id UUID REFERENCES users(id),
    code       TEXT UNIQUE NOT NULL,
    max_uses   INT,
    uses       INT DEFAULT 0,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE bans (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id     UUID REFERENCES spaces(id) ON DELETE CASCADE,
    user_id      UUID REFERENCES users(id),
    moderator_id UUID REFERENCES users(id),
    reason       TEXT,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_invites_code    ON invites(code);
CREATE INDEX idx_bans_space_user ON bans(space_id, user_id);
