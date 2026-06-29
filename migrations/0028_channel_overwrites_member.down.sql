DROP INDEX IF EXISTS idx_overwrite_channel_member;
DROP INDEX IF EXISTS idx_overwrite_channel_role;

DELETE FROM channel_overwrites WHERE member_id IS NOT NULL;

ALTER TABLE channel_overwrites
    DROP CONSTRAINT chk_overwrite_target,
    DROP COLUMN member_id,
    ALTER COLUMN role_id SET NOT NULL,
    ADD CONSTRAINT channel_overwrites_pkey PRIMARY KEY (channel_id, role_id);
