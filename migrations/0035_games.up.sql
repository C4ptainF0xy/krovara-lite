CREATE TABLE games (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    cover_key    TEXT,
    status       TEXT NOT NULL DEFAULT 'pending',
    submitted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    reject_reason TEXT,
    aliases      TEXT[] NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_games_name ON games (lower(name));
CREATE INDEX idx_games_status ON games (status);
