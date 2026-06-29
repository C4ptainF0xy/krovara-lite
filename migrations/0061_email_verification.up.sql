ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;
UPDATE users SET email_verified = true;

CREATE TABLE email_verifications (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
