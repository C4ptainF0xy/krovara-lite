-- name: CreateEvent :one
INSERT INTO events (space_id, title, description, location, starts_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetEvent :one
SELECT * FROM events WHERE id = $1;

-- name: ListSpaceEvents :many
SELECT * FROM events WHERE space_id = $1 ORDER BY starts_at LIMIT 200;

-- name: DeleteEvent :exec
DELETE FROM events WHERE id = $1;

-- name: SetRsvp :exec
INSERT INTO event_rsvps (event_id, user_id, status)
VALUES ($1, $2, $3)
ON CONFLICT (event_id, user_id)
DO UPDATE SET status = EXCLUDED.status, responded_at = NOW();

-- name: GetMyRsvp :one
SELECT status FROM event_rsvps WHERE event_id = $1 AND user_id = $2;

-- name: RsvpCounts :many
SELECT status, COUNT(*) AS n FROM event_rsvps WHERE event_id = $1 GROUP BY status;
