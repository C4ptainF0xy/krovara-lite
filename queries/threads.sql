-- name: CreateThread :one
INSERT INTO threads (channel_id, root_archive_id, title, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetThread :one
SELECT * FROM threads WHERE id = $1;

-- name: ListChannelThreads :many
SELECT sqlc.embed(t),
       (s.user_id IS NOT NULL)::bool AS is_subscribed
  FROM threads t
  LEFT JOIN thread_subscriptions s
         ON s.thread_id = t.id AND s.user_id = sqlc.arg(viewer_id)
 WHERE t.channel_id = sqlc.arg(channel_id)
 ORDER BY t.last_activity_at DESC;

-- name: TouchThreadActivity :one
UPDATE threads SET last_activity_at = NOW() WHERE id = $1 RETURNING *;

-- name: SubscribeThread :exec
INSERT INTO thread_subscriptions (user_id, thread_id)
VALUES ($1, $2)
ON CONFLICT (user_id, thread_id) DO NOTHING;

-- name: UnsubscribeThread :exec
DELETE FROM thread_subscriptions WHERE user_id = $1 AND thread_id = $2;
