CREATE TABLE channel_read_state (
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id           UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    last_read_sort_id    BIGINT NOT NULL DEFAULT 0,
    last_read_archive_id TEXT NOT NULL DEFAULT '',
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, channel_id)
);
