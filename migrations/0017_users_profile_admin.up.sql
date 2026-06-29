ALTER TABLE users
    ADD COLUMN display_name TEXT,
    ADD COLUMN status       TEXT,
    ADD COLUMN is_admin     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN disabled     BOOLEAN NOT NULL DEFAULT false;
