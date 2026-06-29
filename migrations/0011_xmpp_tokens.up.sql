CREATE TABLE xmpp_tokens (
    token      TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_xmpp_tokens_user    ON xmpp_tokens(user_id);
CREATE INDEX idx_xmpp_tokens_expires ON xmpp_tokens(expires_at);
