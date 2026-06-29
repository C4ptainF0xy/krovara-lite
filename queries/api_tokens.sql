-- name: CreateAPIToken :one
INSERT INTO api_tokens (user_id, name, token_hash, prefix, scopes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAPITokenByHash :one
SELECT * FROM api_tokens WHERE token_hash = $1;

-- name: ListUserAPITokens :many
SELECT * FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC;

-- name: TouchAPIToken :exec
UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1;

-- name: DeleteAPIToken :exec
DELETE FROM api_tokens WHERE id = $1 AND user_id = $2;
