-- name: CreateSavedMessage :one
INSERT INTO saved_messages (user_id, channel_id, archive_id, folder)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, archive_id)
    DO UPDATE SET folder = EXCLUDED.folder, channel_id = EXCLUDED.channel_id
RETURNING *;

-- name: ListSavedMessages :many
SELECT * FROM saved_messages
 WHERE user_id = $1
 ORDER BY created_at DESC;

-- name: DeleteSavedMessage :execrows
DELETE FROM saved_messages
 WHERE user_id = $1 AND archive_id = $2;
