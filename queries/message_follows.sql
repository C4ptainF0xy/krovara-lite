-- name: FollowMessage :exec
INSERT INTO message_follows (user_id, channel_id, archive_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: UnfollowMessage :exec
DELETE FROM message_follows
 WHERE user_id = $1 AND channel_id = $2 AND archive_id = $3;

-- name: IsFollowingMessage :one
SELECT EXISTS (
  SELECT 1 FROM message_follows
   WHERE user_id = $1 AND channel_id = $2 AND archive_id = $3
) AS following;

-- name: ListMessageFollowers :many

SELECT user_id FROM message_follows
 WHERE channel_id = $1 AND archive_id = $2;
