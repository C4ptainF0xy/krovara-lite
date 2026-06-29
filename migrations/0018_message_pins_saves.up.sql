CREATE TABLE message_pins (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    archive_id TEXT NOT NULL,
    pinned_by  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (channel_id, archive_id)
);

CREATE INDEX idx_message_pins_channel ON message_pins (channel_id, created_at DESC);

CREATE TABLE saved_messages (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    archive_id TEXT NOT NULL,
    folder     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, archive_id)
);

CREATE INDEX idx_saved_messages_user ON saved_messages (user_id, created_at DESC);
