-- name: CreateFriendRequest :one
INSERT INTO friendships (requester_id, addressee_id, status)
VALUES ($1, $2, 'pending')
RETURNING *;

-- name: GetFriendshipByPair :one

SELECT * FROM friendships
 WHERE (requester_id = $1 AND addressee_id = $2)
    OR (requester_id = $2 AND addressee_id = $1);

-- name: GetFriendshipByID :one
SELECT * FROM friendships WHERE id = $1;

-- name: AcceptFriendRequest :one
UPDATE friendships SET status = 'accepted'
 WHERE id = $1 AND addressee_id = $2 AND status = 'pending'
RETURNING *;

-- name: DeleteFriendship :exec

DELETE FROM friendships
 WHERE id = $1 AND (requester_id = $2 OR addressee_id = $2);

-- name: DeleteFriendshipByPair :exec
DELETE FROM friendships
 WHERE (requester_id = $1 AND addressee_id = $2)
    OR (requester_id = $2 AND addressee_id = $1);

-- name: ListAcceptedFriends :many

SELECT u.id, u.username, u.avatar_key, f.created_at
  FROM friendships f
  JOIN users u ON u.id = CASE WHEN f.requester_id = $1 THEN f.addressee_id ELSE f.requester_id END
 WHERE (f.requester_id = $1 OR f.addressee_id = $1) AND f.status = 'accepted'
 ORDER BY u.username;

-- name: ListIncomingRequests :many
SELECT f.id, u.id AS user_id, u.username, u.avatar_key, f.created_at
  FROM friendships f
  JOIN users u ON u.id = f.requester_id
 WHERE f.addressee_id = $1 AND f.status = 'pending'
 ORDER BY f.created_at DESC;

-- name: ListOutgoingRequests :many
SELECT f.id, u.id AS user_id, u.username, u.avatar_key, f.created_at
  FROM friendships f
  JOIN users u ON u.id = f.addressee_id
 WHERE f.requester_id = $1 AND f.status = 'pending'
 ORDER BY f.created_at DESC;

-- name: CreateBlock :exec
INSERT INTO blocks (blocker_id, blocked_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DeleteBlock :exec
DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2;

-- name: IsBlockedEitherWay :one

SELECT EXISTS (
  SELECT 1 FROM blocks
   WHERE (blocker_id = $1 AND blocked_id = $2)
      OR (blocker_id = $2 AND blocked_id = $1)
) AS blocked;

-- name: ListBlocks :many
SELECT u.id, u.username, u.avatar_key, b.created_at
  FROM blocks b
  JOIN users u ON u.id = b.blocked_id
 WHERE b.blocker_id = $1
 ORDER BY u.username;

-- name: SetWhoCanAdd :exec
UPDATE users SET who_can_add = $2 WHERE id = $1;

-- name: HasMutualFriend :one

WITH a_friends AS (
  SELECT CASE WHEN fa.requester_id = $1 THEN fa.addressee_id ELSE fa.requester_id END AS fid
    FROM friendships fa
   WHERE (fa.requester_id = $1 OR fa.addressee_id = $1) AND fa.status = 'accepted'
),
b_friends AS (
  SELECT CASE WHEN fb.requester_id = $2 THEN fb.addressee_id ELSE fb.requester_id END AS fid
    FROM friendships fb
   WHERE (fb.requester_id = $2 OR fb.addressee_id = $2) AND fb.status = 'accepted'
)
SELECT EXISTS (
  SELECT 1 FROM a_friends a JOIN b_friends b ON a.fid = b.fid
) AS mutual;

-- name: ListMutualFriends :many

SELECT u.id, u.username, COALESCE(u.display_name, u.username) AS display_name, u.avatar_key
  FROM users u
 WHERE u.disabled = false
   AND u.id IN (
     SELECT CASE WHEN f.requester_id = $1 THEN f.addressee_id ELSE f.requester_id END
       FROM friendships f
      WHERE (f.requester_id = $1 OR f.addressee_id = $1) AND f.status = 'accepted'
   )
   AND u.id IN (
     SELECT CASE WHEN f.requester_id = $2 THEN f.addressee_id ELSE f.requester_id END
       FROM friendships f
      WHERE (f.requester_id = $2 OR f.addressee_id = $2) AND f.status = 'accepted'
   )
 ORDER BY display_name;
