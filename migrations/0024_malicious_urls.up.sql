CREATE TABLE malicious_urls (
    url_hash TEXT PRIMARY KEY,
    threat   TEXT,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
