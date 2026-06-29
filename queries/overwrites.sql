-- name: GetMemberChannelOverwrite :one

SELECT * FROM channel_overwrites
 WHERE channel_id = $1 AND member_id = $2;

-- name: ListChannelOverwrites :many

SELECT * FROM channel_overwrites
 WHERE channel_id = $1;

-- name: UpsertRoleOverwrite :one
INSERT INTO channel_overwrites (channel_id, role_id, allow, deny)
VALUES ($1, $2, $3, $4)
ON CONFLICT (channel_id, role_id) WHERE role_id IS NOT NULL
DO UPDATE SET allow = EXCLUDED.allow, deny = EXCLUDED.deny
RETURNING *;

-- name: UpsertMemberOverwrite :one
INSERT INTO channel_overwrites (channel_id, member_id, allow, deny)
VALUES ($1, $2, $3, $4)
ON CONFLICT (channel_id, member_id) WHERE member_id IS NOT NULL
DO UPDATE SET allow = EXCLUDED.allow, deny = EXCLUDED.deny
RETURNING *;

-- name: DeleteRoleOverwrite :exec
DELETE FROM channel_overwrites WHERE channel_id = $1 AND role_id = $2;

-- name: DeleteMemberOverwrite :exec
DELETE FROM channel_overwrites WHERE channel_id = $1 AND member_id = $2;
