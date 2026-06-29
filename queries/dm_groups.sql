-- name: CreateDMGroup :one
INSERT INTO dm_groups (owner_id, name) VALUES ($1, $2) RETURNING *;

-- name: GetDMGroup :one
SELECT * FROM dm_groups WHERE id = $1;

-- name: UpdateDMGroup :one
UPDATE dm_groups
   SET name     = COALESCE(sqlc.narg(name), name),
       icon_key = COALESCE(sqlc.narg(icon_key), icon_key)
 WHERE id = $1
RETURNING *;

-- name: TransferDMGroup :exec
UPDATE dm_groups SET owner_id = $2 WHERE id = $1;

-- name: DeleteDMGroup :exec
DELETE FROM dm_groups WHERE id = $1;

-- name: AddDMGroupMember :exec
INSERT INTO dm_group_members (group_id, user_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveDMGroupMember :exec
DELETE FROM dm_group_members WHERE group_id = $1 AND user_id = $2;

-- name: IsDMGroupMember :one
SELECT EXISTS (SELECT 1 FROM dm_group_members WHERE group_id = $1 AND user_id = $2);

-- name: CountDMGroupMembers :one
SELECT COUNT(*) FROM dm_group_members WHERE group_id = $1;

-- name: ListDMGroupMembers :many
SELECT u.id, u.username, COALESCE(u.display_name, u.username) AS display_name, u.avatar_key, m.joined_at
  FROM dm_group_members m
  JOIN users u ON u.id = m.user_id
 WHERE m.group_id = $1
 ORDER BY m.joined_at;

-- name: ListMyDMGroups :many
SELECT g.id, g.name, g.icon_key, g.owner_id, g.created_at,
       (SELECT COUNT(*) FROM dm_group_members mm WHERE mm.group_id = g.id) AS member_count
  FROM dm_groups g
  JOIN dm_group_members m ON m.group_id = g.id
 WHERE m.user_id = $1
 ORDER BY g.created_at DESC;

-- name: ListMutualDMGroups :many
SELECT g.id, g.name, g.icon_key
  FROM dm_groups g
 WHERE g.id IN (SELECT m.group_id FROM dm_group_members m WHERE m.user_id = $1)
   AND g.id IN (SELECT m.group_id FROM dm_group_members m WHERE m.user_id = $2)
 ORDER BY g.created_at DESC;

-- name: CreateDMGroupMessage :one
INSERT INTO dm_group_messages (group_id, author_id, body)
VALUES ($1, $2, $3) RETURNING *;

-- name: ListDMGroupMessages :many
SELECT m.id, m.group_id, m.author_id, m.body, m.created_at,
       u.username, u.avatar_key
  FROM dm_group_messages m
  JOIN users u ON u.id = m.author_id
 WHERE m.group_id = $1 AND (sqlc.narg(before)::timestamptz IS NULL OR m.created_at < sqlc.narg(before))
 ORDER BY m.created_at DESC
 LIMIT $2;

-- name: CreateDMGroupInvite :exec
INSERT INTO dm_group_invites (code, group_id, created_by) VALUES ($1, $2, $3);

-- name: GetDMGroupInvite :one
SELECT * FROM dm_group_invites WHERE code = $1;

-- name: ListDMGroupInvites :many
SELECT code, created_at FROM dm_group_invites WHERE group_id = $1 ORDER BY created_at DESC;

-- name: DeleteDMGroupInvite :exec
DELETE FROM dm_group_invites WHERE code = $1 AND group_id = $2;
