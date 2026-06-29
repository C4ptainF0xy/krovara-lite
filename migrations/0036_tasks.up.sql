CREATE TABLE tasks (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id          UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    channel_id        UUID REFERENCES channels(id) ON DELETE SET NULL,
    source_archive_id TEXT,
    title             TEXT NOT NULL,
    assignee_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    due_at            TIMESTAMPTZ,
    status            TEXT NOT NULL DEFAULT 'open',
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_space ON tasks(space_id, status, created_at DESC);
