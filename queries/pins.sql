-- name: CreatePin :one
INSERT INTO message_pins (channel_id, archive_id, pinned_by, note)
VALUES ($1, $2, $3, $4)
ON CONFLICT (channel_id, archive_id)
    DO UPDATE SET note = EXCLUDED.note
RETURNING *;

-- name: ListChannelPins :many
SELECT * FROM message_pins
 WHERE channel_id = $1
 ORDER BY created_at DESC;

-- name: DeletePin :execrows
DELETE FROM message_pins
 WHERE channel_id = $1 AND archive_id = $2;
