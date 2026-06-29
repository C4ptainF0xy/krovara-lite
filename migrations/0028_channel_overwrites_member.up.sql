ALTER TABLE channel_overwrites
    DROP CONSTRAINT channel_overwrites_pkey,
    ALTER COLUMN role_id DROP NOT NULL,
    ADD COLUMN member_id UUID REFERENCES members(id) ON DELETE CASCADE,
    ADD CONSTRAINT chk_overwrite_target CHECK (
        (role_id IS NOT NULL AND member_id IS NULL) OR
        (role_id IS NULL AND member_id IS NOT NULL)
    );

CREATE UNIQUE INDEX idx_overwrite_channel_role
    ON channel_overwrites(channel_id, role_id) WHERE role_id IS NOT NULL;
CREATE UNIQUE INDEX idx_overwrite_channel_member
    ON channel_overwrites(channel_id, member_id) WHERE member_id IS NOT NULL;
