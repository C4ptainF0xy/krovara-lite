DROP INDEX IF EXISTS idx_spaces_vanity;
ALTER TABLE spaces
    DROP COLUMN description,
    DROP COLUMN rules,
    DROP COLUMN banner_key,
    DROP COLUMN tags,
    DROP COLUMN language,
    DROP COLUMN vanity_slug;
