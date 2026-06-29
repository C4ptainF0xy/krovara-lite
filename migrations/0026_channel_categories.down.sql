DROP INDEX IF EXISTS idx_channels_category_position;
ALTER TABLE channels DROP COLUMN IF EXISTS category_id;
DROP TABLE IF EXISTS categories;
