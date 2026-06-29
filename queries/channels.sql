-- name: CreateChannel :one
INSERT INTO channels (space_id, name, topic, type, position, is_private, category_id)
VALUES ($1, $2, $3, $4, $5, $6, sqlc.narg(category_id))
RETURNING *;

-- name: GetChannel :one
SELECT * FROM channels WHERE id = $1;

-- name: ListSpaceChannels :many
SELECT * FROM channels
 WHERE space_id = $1
 ORDER BY position, created_at;

-- name: UpdateChannel :one
UPDATE channels
   SET name             = COALESCE(sqlc.narg(name),             name),
       topic            = COALESCE(sqlc.narg(topic),            topic),
       position         = COALESCE(sqlc.narg(position),         position),
       is_private       = COALESCE(sqlc.narg(is_private),       is_private),
       slowmode_seconds = COALESCE(sqlc.narg(slowmode_seconds), slowmode_seconds),
       nsfw             = COALESCE(sqlc.narg(nsfw),             nsfw),
       read_only        = COALESCE(sqlc.narg(read_only),        read_only),
       icon_emoji       = COALESCE(sqlc.narg(icon_emoji),       icon_emoji)
 WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetChannelLock :one
UPDATE channels
   SET locked    = sqlc.arg(locked),
       locked_by = CASE WHEN sqlc.arg(locked) THEN sqlc.arg(locked_by)::uuid ELSE NULL END,
       locked_at = CASE WHEN sqlc.arg(locked) THEN NOW() ELSE NULL END
 WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MoveChannel :one

UPDATE channels
   SET category_id = sqlc.narg(category_id),
       position    = sqlc.arg(position)
 WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteChannel :exec
DELETE FROM channels WHERE id = $1;
