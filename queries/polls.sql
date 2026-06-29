-- name: CreatePoll :one
INSERT INTO polls (space_id, channel_id, question, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreatePollOption :one
INSERT INTO poll_options (poll_id, label, position)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPoll :one
SELECT * FROM polls WHERE id = $1;

-- name: ListChannelPolls :many
SELECT * FROM polls WHERE channel_id = $1 ORDER BY created_at DESC LIMIT 100;

-- name: ListPollOptions :many
SELECT * FROM poll_options WHERE poll_id = $1 ORDER BY position, id;

-- name: PollResults :many
SELECT o.id AS option_id, COUNT(v.user_id) AS votes
  FROM poll_options o
  LEFT JOIN poll_votes v ON v.option_id = o.id
 WHERE o.poll_id = $1
 GROUP BY o.id;

-- name: GetMyVote :one
SELECT option_id FROM poll_votes WHERE poll_id = $1 AND user_id = $2;

-- name: CastVote :exec
INSERT INTO poll_votes (poll_id, option_id, user_id)
VALUES ($1, $2, $3)
ON CONFLICT (poll_id, user_id)
DO UPDATE SET option_id = EXCLUDED.option_id, voted_at = NOW();

-- name: OptionInPoll :one
SELECT EXISTS (SELECT 1 FROM poll_options WHERE id = $1 AND poll_id = $2) AS ok;

-- name: ClosePoll :one
UPDATE polls SET closed = TRUE WHERE id = $1 RETURNING *;
