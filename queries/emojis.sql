-- name: CreateCustomEmoji :one
INSERT INTO custom_emojis (space_id, name, file_key, animated, created_by, kind)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListSpaceEmojis :many
SELECT * FROM custom_emojis WHERE space_id = $1 AND kind = $2 ORDER BY name;

-- name: ListUserEmojis :many
SELECT e.* FROM custom_emojis e
  JOIN members m ON m.space_id = e.space_id
 WHERE m.user_id = $1 AND e.kind = $2
 ORDER BY e.space_id, e.name;

-- name: GetCustomEmoji :one
SELECT * FROM custom_emojis WHERE id = $1;

-- name: DeleteCustomEmoji :exec
DELETE FROM custom_emojis WHERE id = $1;

-- name: CountSpaceEmojis :one
SELECT COUNT(*) FROM custom_emojis WHERE space_id = $1 AND kind = $2;
