-- name: GetKarma :one
SELECT score FROM karma WHERE user_id = $1 AND space_id = $2;

-- name: SumUserKarma :one
SELECT COALESCE(SUM(score), 0)::bigint FROM karma WHERE user_id = $1;

-- name: ListSpaceKarma :many
SELECT user_id, score FROM karma
 WHERE space_id = $1
 ORDER BY score DESC, user_id
 LIMIT $2 OFFSET $3;

-- name: AddKarma :one
INSERT INTO karma (user_id, space_id, score)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, space_id)
DO UPDATE SET score = karma.score + EXCLUDED.score, updated_at = NOW()
RETURNING score;

-- name: InsertKarmaEvent :one
INSERT INTO karma_events (space_id, target_user, source_user, delta, reason)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: CountKarmaEventsBySourceSince :one
SELECT COUNT(*) FROM karma_events
 WHERE source_user = $1 AND created_at >= $2;
