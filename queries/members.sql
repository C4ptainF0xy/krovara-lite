-- name: CreateMember :one
INSERT INTO members (space_id, user_id, nickname)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetMember :one
SELECT * FROM members WHERE id = $1;

-- name: GetMemberByUser :one
SELECT * FROM members
 WHERE space_id = $1 AND user_id = $2;

-- name: DeleteMember :exec
DELETE FROM members WHERE id = $1;

-- name: UpdateMemberNickname :one
UPDATE members SET nickname = $2 WHERE id = $1 RETURNING *;

-- name: ListSpaceMembers :many

SELECT m.id, m.space_id, m.user_id, m.nickname, m.joined_at,
       u.username,
       COALESCE(m.avatar_key, u.avatar_key) AS avatar_key,
       m.bio,
       u.badges,
       (SELECT r.color FROM member_roles mr JOIN roles r ON r.id = mr.role_id
         WHERE mr.member_id = m.id AND r.color IS NOT NULL AND r.is_everyone IS NOT TRUE
         ORDER BY r.position DESC LIMIT 1) AS role_color,
       (SELECT r.icon_emoji FROM member_roles mr JOIN roles r ON r.id = mr.role_id
         WHERE mr.member_id = m.id AND r.icon_emoji IS NOT NULL AND r.is_everyone IS NOT TRUE
         ORDER BY r.position DESC LIMIT 1) AS role_icon,

       COALESCE((SELECT r.name FROM member_roles mr JOIN roles r ON r.id = mr.role_id
         WHERE mr.member_id = m.id AND r.hoist IS TRUE AND r.is_everyone IS NOT TRUE
         ORDER BY r.position DESC LIMIT 1), '')::text AS hoist_role,
       COALESCE((SELECT r.position FROM member_roles mr JOIN roles r ON r.id = mr.role_id
         WHERE mr.member_id = m.id AND r.hoist IS TRUE AND r.is_everyone IS NOT TRUE
         ORDER BY r.position DESC LIMIT 1), 0)::int AS hoist_position
  FROM members m
  JOIN users u ON u.id = m.user_id
 WHERE m.space_id = $1
 ORDER BY m.joined_at;

-- name: UpdateMemberSpaceProfile :one
UPDATE members
   SET nickname   = $3,
       avatar_key = $4,
       bio        = $5
 WHERE space_id = $1 AND user_id = $2
RETURNING *;

-- name: ListMutualSpaces :many

SELECT s.id, s.name, s.icon_key
  FROM spaces s
 WHERE s.id IN (SELECT m.space_id FROM members m WHERE m.user_id = $1)
   AND s.id IN (SELECT m.space_id FROM members m WHERE m.user_id = $2)
 ORDER BY s.name;
