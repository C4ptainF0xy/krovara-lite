-- name: SubmitGame :one
INSERT INTO games (name, cover_key, submitted_by, aliases)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetGame :one
SELECT * FROM games WHERE id = $1;

-- name: ListApprovedGames :many

SELECT * FROM games
 WHERE status = 'approved'
   AND ($1::text = '' OR name ILIKE '%' || $1 || '%')
 ORDER BY name
 LIMIT $2;

-- name: ListGamesByStatus :many

SELECT * FROM games WHERE status = $1 ORDER BY created_at LIMIT $2;

-- name: ReviewGame :one

UPDATE games
   SET status = $2, reviewed_by = $3, reject_reason = sqlc.narg('reject_reason')
 WHERE id = $1
RETURNING *;
