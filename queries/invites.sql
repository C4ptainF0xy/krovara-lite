-- name: CreateInvite :one
INSERT INTO invites (space_id, creator_id, code, max_uses, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetInviteByCode :one
SELECT * FROM invites WHERE code = $1;

-- name: FindReusableInvite :one

SELECT * FROM invites
 WHERE space_id = $1
   AND max_uses IS NOT DISTINCT FROM sqlc.narg('max_uses')::int
   AND (expires_at IS NULL OR expires_at > NOW())
   AND (max_uses IS NULL OR COALESCE(uses, 0) < max_uses)
 ORDER BY created_at DESC
 LIMIT 1;

-- name: ListSpaceInvites :many
SELECT * FROM invites WHERE space_id = $1 ORDER BY created_at DESC;

-- name: DeleteInvite :exec
DELETE FROM invites WHERE id = $1;

-- name: DeleteInviteByCode :exec
DELETE FROM invites WHERE code = $1;

-- name: IncrementInviteUses :one
UPDATE invites SET uses = COALESCE(uses, 0) + 1
 WHERE id = $1
RETURNING *;

-- name: DeleteExpiredInvites :exec
DELETE FROM invites
 WHERE (expires_at IS NOT NULL AND expires_at < NOW())
    OR (max_uses IS NOT NULL AND COALESCE(uses, 0) >= max_uses);
