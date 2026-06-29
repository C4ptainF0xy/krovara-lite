CREATE TABLE threads (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id       UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    root_archive_id  TEXT NOT NULL,
    title            TEXT NOT NULL,
    created_by       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_threads_channel ON threads (channel_id, last_activity_at DESC);

CREATE TABLE thread_subscriptions (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    thread_id  UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, thread_id)
);

CREATE INDEX idx_thread_subscriptions_thread ON thread_subscriptions (thread_id, user_id);
