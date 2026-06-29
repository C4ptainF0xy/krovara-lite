-- name: CreateFile :one
INSERT INTO files (owner_id, filename, size, mimetype, path, sha256, kind)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetFile :one
SELECT * FROM files WHERE id = $1;

-- name: GetFileByOwnerSHA :one

SELECT * FROM files
 WHERE owner_id = $1 AND sha256 = $2;

-- name: DeleteFile :exec
DELETE FROM files WHERE id = $1;

-- name: UpdateFileScanStatus :exec

UPDATE files SET scan_status = $2 WHERE id = $1;

-- name: ListOwnerFilesByKind :many
SELECT * FROM files
 WHERE owner_id = $1 AND kind = $2
 ORDER BY created_at DESC;

-- name: TotalOwnerStorage :one
SELECT COALESCE(SUM(size), 0)::bigint FROM files WHERE owner_id = $1;

-- name: ListOwnerFiles :many
SELECT id, filename, size, mimetype, kind, created_at
  FROM files
 WHERE owner_id = $1
 ORDER BY created_at DESC
 LIMIT 200;
