-- name: CreateXMPPToken :one
INSERT INTO xmpp_tokens (token, user_id, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ConsumeXMPPToken :one

DELETE FROM xmpp_tokens
 WHERE token = $1 AND expires_at > NOW()
RETURNING *;

-- name: DeleteExpiredXMPPTokens :exec
DELETE FROM xmpp_tokens WHERE expires_at < NOW();
