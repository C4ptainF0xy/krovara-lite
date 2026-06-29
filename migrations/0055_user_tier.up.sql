ALTER TABLE users ADD COLUMN tier TEXT NOT NULL DEFAULT 'free'
    CHECK (tier IN ('free', 'plus'));
