-- name: CreateWebhook :one
INSERT INTO webhooks (space_id, channel_id, name, url, secret, events)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetWebhook :one
SELECT * FROM webhooks WHERE id = $1;

-- name: ListSpaceWebhooks :many
SELECT * FROM webhooks WHERE space_id = $1 ORDER BY created_at DESC;

-- name: ListWebhooksForEvent :many

SELECT * FROM webhooks
 WHERE space_id = $1
   AND sqlc.arg(event)::TEXT = ANY(events);

-- name: DeleteWebhook :exec
DELETE FROM webhooks WHERE id = $1;

-- name: UpdateWebhook :one
UPDATE webhooks
   SET name       = COALESCE(sqlc.narg(name),       name),
       url        = COALESCE(sqlc.narg(url),        url),
       channel_id = COALESCE(sqlc.narg(channel_id), channel_id),
       events     = COALESCE(sqlc.narg(events),     events)
 WHERE id = sqlc.arg(id)
RETURNING *;
