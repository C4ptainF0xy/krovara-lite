CREATE TABLE message_follows (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    archive_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, channel_id, archive_id)
);

CREATE INDEX idx_message_follows_target ON message_follows (channel_id, archive_id);
