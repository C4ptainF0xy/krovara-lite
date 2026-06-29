-- name: GetNotifSetting :one
SELECT * FROM notif_settings
 WHERE user_id = $1 AND scope_type = $2 AND scope_id = $3;

-- name: UpsertNotifSetting :one
INSERT INTO notif_settings (user_id, scope_type, scope_id, level, muted_until, suppress_everyone)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, scope_type, scope_id)
DO UPDATE SET level = EXCLUDED.level,
              muted_until = EXCLUDED.muted_until,
              suppress_everyone = EXCLUDED.suppress_everyone
RETURNING *;

-- name: ListNotifSettings :many
SELECT * FROM notif_settings WHERE user_id = $1;

-- name: DeleteNotifSetting :exec
DELETE FROM notif_settings
 WHERE user_id = $1 AND scope_type = $2 AND scope_id = $3;

-- name: CreateInboxItem :exec
INSERT INTO inbox_items (user_id, kind, space_id, channel_id, archive_id, author_id, preview)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id, channel_id, archive_id) DO NOTHING;

-- name: ListInbox :many
SELECT * FROM inbox_items
 WHERE user_id = $1
 ORDER BY created_at DESC
 LIMIT $2;

-- name: CountInboxUnread :one
SELECT COUNT(*) FROM inbox_items WHERE user_id = $1 AND read = FALSE;

-- name: MarkInboxItemRead :exec
UPDATE inbox_items SET read = TRUE WHERE id = $1 AND user_id = $2;

-- name: MarkAllInboxRead :exec
UPDATE inbox_items SET read = TRUE WHERE user_id = $1 AND read = FALSE;
