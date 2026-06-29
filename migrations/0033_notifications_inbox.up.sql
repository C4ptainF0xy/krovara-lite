CREATE TABLE notif_settings (
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope_type        TEXT NOT NULL,
    scope_id          UUID NOT NULL,
    level             TEXT NOT NULL DEFAULT 'all',
    muted_until       TIMESTAMPTZ,
    suppress_everyone BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (user_id, scope_type, scope_id)
);

CREATE TABLE inbox_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    space_id    UUID REFERENCES spaces(id) ON DELETE CASCADE,
    channel_id  UUID REFERENCES channels(id) ON DELETE CASCADE,
    archive_id  TEXT NOT NULL,
    author_id   UUID,
    preview     TEXT,
    read        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_inbox_user ON inbox_items(user_id, created_at DESC);
CREATE INDEX idx_inbox_user_unread ON inbox_items(user_id) WHERE read = FALSE;

CREATE UNIQUE INDEX idx_inbox_dedup ON inbox_items(user_id, channel_id, archive_id);
