ALTER TABLE custom_emojis
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'emoji'
    CHECK (kind IN ('emoji', 'sticker'));

ALTER TABLE custom_emojis DROP CONSTRAINT custom_emojis_space_id_name_key;
ALTER TABLE custom_emojis
    ADD CONSTRAINT custom_emojis_space_kind_name_key UNIQUE (space_id, kind, name);
