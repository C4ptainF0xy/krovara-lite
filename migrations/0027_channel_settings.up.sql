ALTER TABLE channels
    ADD COLUMN slowmode_seconds INT NOT NULL DEFAULT 0,
    ADD COLUMN nsfw             BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN read_only        BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN icon_emoji       TEXT;
