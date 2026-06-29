CREATE TABLE categories (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id   UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    position   INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_categories_space_position ON categories(space_id, position);

ALTER TABLE channels
    ADD COLUMN category_id UUID REFERENCES categories(id) ON DELETE SET NULL;

CREATE INDEX idx_channels_category_position ON channels(category_id, position);
