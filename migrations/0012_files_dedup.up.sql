ALTER TABLE files
    ADD COLUMN sha256 TEXT NOT NULL DEFAULT '',
    ADD COLUMN kind   TEXT NOT NULL DEFAULT 'attachment';

CREATE UNIQUE INDEX idx_files_owner_sha256 ON files(owner_id, sha256)
    WHERE sha256 <> '';

CREATE INDEX idx_files_kind ON files(kind);
