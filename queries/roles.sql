-- name: GetRole :one
SELECT * FROM roles WHERE id = $1;

-- name: UpdateRole :one
UPDATE roles
   SET name        = COALESCE(sqlc.narg(name),        name),
       permissions = COALESCE(sqlc.narg(permissions), permissions),
       color       = COALESCE(sqlc.narg(color),       color),
       position    = COALESCE(sqlc.narg(position),    position),
       hoist       = COALESCE(sqlc.narg(hoist),       hoist),
       mentionable = COALESCE(sqlc.narg(mentionable), mentionable),
       icon_emoji  = COALESCE(sqlc.narg(icon_emoji),  icon_emoji)
 WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteRole :exec
DELETE FROM roles WHERE id = $1 AND is_everyone = FALSE;

-- name: GetMemberMaxRolePosition :one

SELECT COALESCE(MAX(r.position), 0)::INT AS max_position
  FROM member_roles mr
  JOIN roles r ON r.id = mr.role_id
 WHERE mr.member_id = $1;

-- name: CreateRole :one
INSERT INTO roles (space_id, name, permissions, color, position, is_everyone)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetEveryoneRole :one
SELECT * FROM roles
 WHERE space_id = $1 AND is_everyone = TRUE
 LIMIT 1;

-- name: ListSpaceRoles :many
SELECT * FROM roles
 WHERE space_id = $1
 ORDER BY position;

-- name: ListMemberRoles :many
SELECT r.*
  FROM roles r
  JOIN member_roles mr ON mr.role_id = r.id
 WHERE mr.member_id = $1
 ORDER BY r.position;

-- name: ListRoleMembers :many

SELECT m.id AS member_id, m.user_id, u.username, m.nickname
  FROM member_roles mr
  JOIN members m ON m.id = mr.member_id
  JOIN users u ON u.id = m.user_id
 WHERE mr.role_id = $1
 ORDER BY u.username;

-- name: ListSpaceMemberIDs :many

SELECT id FROM members WHERE space_id = $1;

-- name: ListChannelOverwritesForRoles :many
SELECT * FROM channel_overwrites
 WHERE channel_id = $1
   AND role_id = ANY(sqlc.arg(role_ids)::uuid[]);

-- name: ListMentionableRoleMembers :many
SELECT lower(r.name) AS role_name, m.user_id
  FROM roles r
  JOIN member_roles mr ON mr.role_id = r.id
  JOIN members m       ON m.id = mr.member_id
 WHERE r.space_id = $1 AND r.mentionable = TRUE AND r.is_everyone IS NOT TRUE;
