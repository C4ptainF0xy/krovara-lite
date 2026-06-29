-- name: ListSavedSearches :many
SELECT * FROM saved_searches WHERE user_id = $1 ORDER BY created_at DESC;

-- name: CountSavedSearches :one
SELECT COUNT(*) FROM saved_searches WHERE user_id = $1;

-- name: CreateSavedSearch :one
INSERT INTO saved_searches (user_id, space_id, name, query)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteSavedSearch :execrows
DELETE FROM saved_searches WHERE id = $1 AND user_id = $2;
