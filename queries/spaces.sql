-- name: CreateSpace :one
INSERT INTO spaces (owner_id, name, icon_key)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSpace :one
SELECT * FROM spaces WHERE id = $1;

-- name: CountSpaceMembers :one
SELECT COUNT(*) FROM members WHERE space_id = $1;

-- name: UpdateSpace :one
UPDATE spaces
   SET name        = COALESCE(sqlc.narg(name),        name),
       icon_key    = COALESCE(sqlc.narg(icon_key),    icon_key),
       description = COALESCE(sqlc.narg(description), description),
       rules       = COALESCE(sqlc.narg(rules),       rules),
       banner_key  = COALESCE(sqlc.narg(banner_key),  banner_key),
       tags        = COALESCE(sqlc.narg(tags),        tags),
       language    = COALESCE(sqlc.narg(language),    language)
 WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetSpaceVanity :one

UPDATE spaces SET vanity_slug = $2 WHERE id = $1
RETURNING *;

-- name: GetSpaceByVanity :one
SELECT * FROM spaces WHERE lower(vanity_slug) = lower($1);

-- name: TransferSpaceOwnership :one
UPDATE spaces SET owner_id = $2 WHERE id = $1
RETURNING *;

-- name: DeleteSpace :exec
DELETE FROM spaces WHERE id = $1;

-- name: ListUserSpaces :many
SELECT s.*
  FROM spaces s
  JOIN members m ON m.space_id = s.id
 WHERE m.user_id = $1
 ORDER BY s.created_at;
