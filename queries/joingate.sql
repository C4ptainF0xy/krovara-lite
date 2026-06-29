-- name: GetJoinForm :one
SELECT * FROM join_forms WHERE space_id = $1;

-- name: UpsertJoinForm :one
INSERT INTO join_forms (space_id, enabled, questions, auto_role_id, min_karma, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (space_id)
DO UPDATE SET enabled      = EXCLUDED.enabled,
              questions    = EXCLUDED.questions,
              auto_role_id = EXCLUDED.auto_role_id,
              min_karma    = EXCLUDED.min_karma,
              updated_at   = NOW()
RETURNING *;

-- name: CreateJoinRequest :one
INSERT INTO join_requests (space_id, user_id, answers)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetJoinRequest :one
SELECT * FROM join_requests WHERE id = $1;

-- name: ListJoinRequests :many
SELECT jr.*, u.username, u.display_name, u.avatar_key
  FROM join_requests jr
  JOIN users u ON u.id = jr.user_id
 WHERE jr.space_id = $1 AND jr.status = $2
 ORDER BY jr.created_at ASC
 LIMIT $3 OFFSET $4;

-- name: ReviewJoinRequest :one
UPDATE join_requests
   SET status = $2, reviewed_by = $3, reviewed_at = NOW()
 WHERE id = $1 AND status = 'pending'
RETURNING *;
