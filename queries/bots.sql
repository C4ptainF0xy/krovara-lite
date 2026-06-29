-- name: CreateBot :one
INSERT INTO bots (space_id, name, component_jid, secret_hash)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetBot :one
SELECT * FROM bots WHERE id = $1;

-- name: ListSpaceBots :many
SELECT * FROM bots WHERE space_id = $1 ORDER BY created_at DESC;

-- name: DeleteBot :exec
DELETE FROM bots WHERE id = $1;
