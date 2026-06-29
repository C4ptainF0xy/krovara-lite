-- name: UpsertReadState :one

INSERT INTO channel_read_state (user_id, channel_id, last_read_sort_id, last_read_archive_id, updated_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (user_id, channel_id)
    DO UPDATE SET
        last_read_sort_id = CASE
            WHEN sqlc.arg(advance_only)::bool AND channel_read_state.last_read_sort_id >= EXCLUDED.last_read_sort_id
            THEN channel_read_state.last_read_sort_id
            ELSE EXCLUDED.last_read_sort_id END,
        last_read_archive_id = CASE
            WHEN sqlc.arg(advance_only)::bool AND channel_read_state.last_read_sort_id >= EXCLUDED.last_read_sort_id
            THEN channel_read_state.last_read_archive_id
            ELSE EXCLUDED.last_read_archive_id END,
        updated_at = NOW()
RETURNING *;

-- name: ListReadState :many
SELECT * FROM channel_read_state
 WHERE user_id = $1;
