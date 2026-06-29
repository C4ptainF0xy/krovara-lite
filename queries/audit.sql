-- name: CreateAuditLog :one
INSERT INTO audit_logs (space_id, actor_id, action, target_id, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListSpaceAuditLogs :many
SELECT * FROM audit_logs
 WHERE space_id = $1
 ORDER BY created_at DESC
 LIMIT $2;
