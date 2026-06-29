-- name: MaliciousURLHashes :many

SELECT url_hash, threat FROM malicious_urls
 WHERE url_hash = ANY(@hashes::text[]);
