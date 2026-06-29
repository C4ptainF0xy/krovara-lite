ALTER TABLE channels
    DROP COLUMN locked,
    DROP COLUMN locked_by,
    DROP COLUMN locked_at;
