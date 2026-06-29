ALTER TABLE users ADD COLUMN signup_ip_hash TEXT;

CREATE INDEX idx_users_signup_ip_hash ON users(signup_ip_hash)
    WHERE signup_ip_hash IS NOT NULL;
