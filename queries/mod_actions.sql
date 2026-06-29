-- name: CreateModAction :one
INSERT INTO mod_actions (space_id, target_user, moderator_id, action, reason, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetActiveTimeout :one

SELECT * FROM mod_actions
 WHERE space_id = $1 AND target_user = $2 AND action = 'timeout' AND active = TRUE
   AND (expires_at IS NULL OR expires_at > NOW())
 ORDER BY created_at DESC
 LIMIT 1;

-- name: ListSpaceModActions :many

SELECT * FROM mod_actions
 WHERE space_id = $1
 ORDER BY created_at DESC
 LIMIT $2;

-- name: RevokeTimeout :exec

UPDATE mod_actions SET active = FALSE
 WHERE space_id = $1 AND target_user = $2 AND action = 'timeout' AND active = TRUE;

-- name: DeactivateExpiredTimeouts :exec

UPDATE mod_actions SET active = FALSE
 WHERE action = 'timeout' AND active = TRUE
   AND expires_at IS NOT NULL AND expires_at <= NOW();
