CREATE TABLE polls (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id   UUID NOT NULL REFERENCES spaces(id)   ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    question   TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    closed     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE poll_options (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id  UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    label    TEXT NOT NULL,
    position INT  NOT NULL DEFAULT 0
);

CREATE TABLE poll_votes (
    poll_id   UUID NOT NULL REFERENCES polls(id)        ON DELETE CASCADE,
    option_id UUID NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL REFERENCES users(id)        ON DELETE CASCADE,
    voted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (poll_id, user_id)
);

CREATE INDEX idx_polls_channel       ON polls(channel_id, created_at DESC);
CREATE INDEX idx_poll_options_poll   ON poll_options(poll_id, position);
CREATE INDEX idx_poll_votes_option   ON poll_votes(option_id);
