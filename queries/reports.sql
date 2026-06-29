-- name: CreateReport :one
INSERT INTO reports (reporter_id, target_type, target_id, reason, space_id, channel_id, category, context)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetReport :one
SELECT * FROM reports WHERE id = $1;

-- name: ListSpaceReports :many
SELECT * FROM reports
 WHERE space_id = $1
   AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
   AND (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category')::text)
 ORDER BY created_at DESC;

-- name: ListReports :many
SELECT * FROM reports
 WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
 ORDER BY created_at DESC
 LIMIT $1 OFFSET $2;

-- name: ResolveReport :one
UPDATE reports
   SET status = $2, resolved_by = $3, resolved_at = NOW()
 WHERE id = $1
RETURNING *;

-- name: CountPendingReports :one

SELECT COUNT(*) FROM reports
 WHERE space_id = $1 AND status IN ('pending', 'in_progress');

-- name: ClaimReport :one

UPDATE reports
   SET assigned_to = $2, status = 'in_progress'
 WHERE id = $1
RETURNING *;

-- name: ListReportComments :many

SELECT * FROM report_comments WHERE report_id = $1 ORDER BY created_at ASC;

-- name: CreateReportComment :one
INSERT INTO report_comments (report_id, author_id, body)
VALUES ($1, $2, $3)
RETURNING *;
