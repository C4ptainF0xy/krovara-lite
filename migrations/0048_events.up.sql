CREATE TABLE events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id    UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT,
    location    TEXT,
    starts_at   TIMESTAMPTZ NOT NULL,
    created_by  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE event_rsvps (
    event_id  UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    status    TEXT NOT NULL,
    responded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, user_id)
);

CREATE INDEX idx_events_space ON events(space_id, starts_at);
