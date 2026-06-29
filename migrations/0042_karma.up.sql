CREATE TABLE karma (
    user_id    UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    space_id   UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    score      INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, space_id)
);

CREATE TABLE karma_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id    UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    target_user UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    source_user UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    delta       INTEGER NOT NULL,
    reason      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_karma_events_unique_vouch
    ON karma_events(space_id, source_user, target_user);

CREATE INDEX idx_karma_events_source_time
    ON karma_events(source_user, created_at);
