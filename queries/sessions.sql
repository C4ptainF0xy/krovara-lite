-- name: GetSessionByRefreshToken :one
SELECT * FROM sessions WHERE refresh_token = $1;

-- name: GetSessionByID :one
SELECT * FROM sessions WHERE id = $1;

-- name: CreateSession :one
INSERT INTO sessions (user_id, refresh_token, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CreateSessionInFamily :one

INSERT INTO sessions (user_id, refresh_token, expires_at, family_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ConsumeSession :one

UPDATE sessions
   SET replaced_by = sqlc.arg(replaced_by), used_at = NOW()
 WHERE refresh_token = sqlc.arg(old_refresh_token) AND replaced_by IS NULL
RETURNING *;

-- name: DeleteSessionFamily :exec

DELETE FROM sessions WHERE family_id = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteSessionByRefreshToken :exec
DELETE FROM sessions WHERE refresh_token = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < NOW();

-- name: RotateSession :one
UPDATE sessions
   SET refresh_token = sqlc.arg(new_refresh_token),
       expires_at    = sqlc.arg(new_expires_at)
 WHERE refresh_token = sqlc.arg(old_refresh_token)
RETURNING *;
