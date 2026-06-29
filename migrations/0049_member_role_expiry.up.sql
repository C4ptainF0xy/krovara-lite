ALTER TABLE member_roles ADD COLUMN expires_at TIMESTAMPTZ;

CREATE INDEX idx_member_roles_expires ON member_roles (expires_at)
    WHERE expires_at IS NOT NULL;
