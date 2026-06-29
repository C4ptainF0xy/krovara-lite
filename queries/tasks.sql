-- name: CreateTask :one
INSERT INTO tasks (space_id, channel_id, source_archive_id, title, assignee_id, due_at, created_by)
VALUES ($1, sqlc.narg('channel_id'), sqlc.narg('source_archive_id'), $2,
        sqlc.narg('assignee_id'), sqlc.narg('due_at'), $3)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListSpaceTasks :many
SELECT * FROM tasks
 WHERE space_id = $1
 ORDER BY (status = 'done'), created_at DESC
 LIMIT $2;

-- name: UpdateTask :one
UPDATE tasks
   SET title       = COALESCE(sqlc.narg('title'), title),
       status      = COALESCE(sqlc.narg('status'), status),
       assignee_id = COALESCE(sqlc.narg('assignee_id'), assignee_id),
       due_at      = COALESCE(sqlc.narg('due_at'), due_at)
 WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;
