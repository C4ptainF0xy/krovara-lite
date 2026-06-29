ALTER TABLE spaces
    ADD COLUMN description TEXT,
    ADD COLUMN rules       TEXT,
    ADD COLUMN banner_key  TEXT,
    ADD COLUMN tags        TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN language    TEXT,
    ADD COLUMN vanity_slug TEXT;

CREATE UNIQUE INDEX idx_spaces_vanity ON spaces (lower(vanity_slug))
    WHERE vanity_slug IS NOT NULL;
