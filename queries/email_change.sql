-- name: CreateEmailChangeToken :one
INSERT INTO email_change_tokens (user_id, new_email, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: InvalidatePendingEmailChanges :exec
UPDATE email_change_tokens
   SET used_at = NOW()
 WHERE user_id = $1 AND used_at IS NULL;

-- name: ConsumeEmailChangeToken :one
UPDATE email_change_tokens
   SET used_at = NOW()
 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
RETURNING *;

-- name: UpdateUserEmail :exec
UPDATE users SET email = $2 WHERE id = $1;
