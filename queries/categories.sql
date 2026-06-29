-- name: CreateCategory :one
INSERT INTO categories (space_id, name, position)
VALUES ($1, $2, COALESCE(sqlc.narg(position), (
    SELECT COALESCE(MAX(position), -1) + 1 FROM categories WHERE space_id = $1
)))
RETURNING *;

-- name: GetCategory :one
SELECT * FROM categories WHERE id = $1;

-- name: ListSpaceCategories :many
SELECT * FROM categories
 WHERE space_id = $1
 ORDER BY position, created_at;

-- name: UpdateCategory :one
UPDATE categories
   SET name     = COALESCE(sqlc.narg(name),     name),
       position = COALESCE(sqlc.narg(position), position)
 WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteCategory :exec
DELETE FROM categories WHERE id = $1;
