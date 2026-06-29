-- name: UpsertListing :one

INSERT INTO space_listings (space_id, category, member_count, delisted_at)
VALUES ($1, $2, (SELECT COUNT(*) FROM members WHERE space_id = $1), NULL)
ON CONFLICT (space_id)
DO UPDATE SET category = EXCLUDED.category,
              member_count = EXCLUDED.member_count,
              delisted_at = NULL,
              listed_at = NOW()
RETURNING *;

-- name: DelistSpace :exec
UPDATE space_listings SET delisted_at = NOW() WHERE space_id = $1;

-- name: GetListing :one
SELECT * FROM space_listings WHERE space_id = $1;

-- name: ExploreListings :many

SELECT l.space_id, l.category, l.member_count, l.listed_at,
       s.name, s.description, s.icon_key, s.banner_key, s.tags, s.language, s.vanity_slug
  FROM space_listings l
  JOIN spaces s ON s.id = l.space_id
 WHERE l.delisted_at IS NULL
   AND ($1::text = '' OR l.category = $1)
   AND ($2::text = '' OR s.name ILIKE '%' || $2 || '%' OR COALESCE(s.description,'') ILIKE '%' || $2 || '%')
 ORDER BY l.member_count DESC, l.listed_at DESC
 LIMIT $3;
