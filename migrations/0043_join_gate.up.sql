CREATE TABLE join_forms (
    space_id     UUID PRIMARY KEY REFERENCES spaces(id) ON DELETE CASCADE,
    enabled      BOOLEAN NOT NULL DEFAULT false,

    questions    JSONB NOT NULL DEFAULT '[]',

    auto_role_id UUID REFERENCES roles(id) ON DELETE SET NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE join_requests (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id    UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,

    answers     JSONB NOT NULL DEFAULT '[]',
    status      TEXT NOT NULL DEFAULT 'pending',
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_join_requests_pending
    ON join_requests(space_id, user_id) WHERE status = 'pending';

CREATE INDEX idx_join_requests_space_status
    ON join_requests(space_id, status);
