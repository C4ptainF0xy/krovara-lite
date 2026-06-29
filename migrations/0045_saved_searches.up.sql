CREATE TABLE saved_searches (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    space_id   UUID          REFERENCES spaces(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    query      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_saved_searches_user ON saved_searches(user_id, created_at DESC);
