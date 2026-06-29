-- name: CreateDevice :one
INSERT INTO devices (user_id, name, ntfy_topic)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, ntfy_topic) DO UPDATE SET name = EXCLUDED.name
RETURNING *;

-- name: ListUserDevices :many
SELECT * FROM devices WHERE user_id = $1 ORDER BY created_at DESC;

-- name: DeleteDevice :exec
DELETE FROM devices WHERE id = $1 AND user_id = $2;

-- name: UpsertPushPref :one
INSERT INTO push_prefs (user_id, space_id, scope)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, space_id) DO UPDATE SET scope = EXCLUDED.scope
RETURNING *;

-- name: GetPushPref :one
SELECT * FROM push_prefs WHERE user_id = $1 AND space_id = $2;

-- name: ListUserPushPrefs :many
SELECT * FROM push_prefs WHERE user_id = $1;
