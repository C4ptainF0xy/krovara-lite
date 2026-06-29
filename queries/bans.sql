-- name: CreateBan :one
INSERT INTO bans (space_id, user_id, moderator_id, reason)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetBan :one
SELECT * FROM bans
 WHERE space_id = $1 AND user_id = $2;

-- name: ListSpaceBans :many
SELECT * FROM bans WHERE space_id = $1 ORDER BY created_at DESC;

-- name: DeleteBan :exec
DELETE FROM bans
 WHERE space_id = $1 AND user_id = $2;

-- name: IsUserBanned :one
SELECT EXISTS (
  SELECT 1 FROM bans WHERE space_id = $1 AND user_id = $2
) AS banned;
