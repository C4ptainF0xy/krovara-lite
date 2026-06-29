ALTER TABLE users
    DROP COLUMN IF EXISTS disabled,
    DROP COLUMN IF EXISTS is_admin,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS display_name;
