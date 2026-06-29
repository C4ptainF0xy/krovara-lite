CREATE TABLE mod_actions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id     UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    target_user  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    moderator_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action       TEXT NOT NULL,
    reason       TEXT,
    expires_at   TIMESTAMPTZ,
    active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_mod_actions_space ON mod_actions(space_id, created_at DESC);

CREATE INDEX idx_mod_actions_active
    ON mod_actions(space_id, target_user)
    WHERE active AND action = 'timeout';
