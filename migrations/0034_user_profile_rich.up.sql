ALTER TABLE users
    ADD COLUMN banner_key   TEXT,
    ADD COLUMN bio          TEXT,
    ADD COLUMN pronouns     TEXT,
    ADD COLUMN links        JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN accent_color TEXT;
