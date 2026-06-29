CREATE TABLE space_listings (
    space_id     UUID PRIMARY KEY REFERENCES spaces(id) ON DELETE CASCADE,
    category     TEXT NOT NULL DEFAULT 'other',
    member_count INT NOT NULL DEFAULT 0,
    listed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delisted_at  TIMESTAMPTZ
);

CREATE INDEX idx_listings_active ON space_listings (category, member_count DESC)
    WHERE delisted_at IS NULL;
