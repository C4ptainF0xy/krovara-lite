-- name: AssignMemberRole :exec

INSERT INTO member_roles (member_id, role_id, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (member_id, role_id) DO UPDATE SET expires_at = EXCLUDED.expires_at;

-- name: RemoveMemberRole :exec
DELETE FROM member_roles WHERE member_id = $1 AND role_id = $2;

-- name: DeleteExpiredMemberRoles :execrows
DELETE FROM member_roles WHERE expires_at IS NOT NULL AND expires_at < now();

-- name: HasMemberRole :one
SELECT EXISTS (
  SELECT 1 FROM member_roles WHERE member_id = $1 AND role_id = $2
) AS present;
