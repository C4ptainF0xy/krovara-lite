-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE LOWER(username) = LOWER($1);

-- name: CreateUser :one
INSERT INTO users (username, email, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: SetUserSignupIPHash :exec
UPDATE users SET signup_ip_hash = $2 WHERE id = $1;

-- name: CountAccountsBySignupIPHash :one
SELECT COUNT(*) FROM users WHERE signup_ip_hash = $1;

-- name: UpdateUserAvatar :exec
UPDATE users SET avatar_key = $2 WHERE id = $1;

-- name: SetUserBadges :exec
UPDATE users SET badges = $2 WHERE id = $1;

-- name: UpdateUserProfile :one
UPDATE users
   SET display_name = COALESCE(sqlc.narg('display_name'), display_name),
       status       = COALESCE(sqlc.narg('status'), status),
       avatar_key   = COALESCE(sqlc.narg('avatar_key'), avatar_key),
       banner_key   = COALESCE(sqlc.narg('banner_key'), banner_key),
       bio          = COALESCE(sqlc.narg('bio'), bio),
       pronouns     = COALESCE(sqlc.narg('pronouns'), pronouns),
       links        = COALESCE(sqlc.narg('links'), links),
       accent_color = COALESCE(sqlc.narg('accent_color'), accent_color)
 WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: SetUserDisabled :one
UPDATE users SET disabled = $2 WHERE id = $1 RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: GetOAuthAccount :one
SELECT * FROM oauth_accounts
WHERE provider = $1 AND provider_id = $2;

-- name: CreateOAuthAccount :one
INSERT INTO oauth_accounts (user_id, provider, provider_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: SetTOTP :exec
UPDATE users SET totp_secret = $2, totp_enabled = $3, backup_codes = $4 WHERE id = $1;

-- name: DisableTOTP :exec
UPDATE users SET totp_secret = NULL, totp_enabled = false, backup_codes = NULL WHERE id = $1;

-- name: UpdateBackupCodes :exec
UPDATE users SET backup_codes = $2 WHERE id = $1;

-- name: SoftDeleteUser :exec

UPDATE users
SET username = $2, email = $3, password_hash = NULL, avatar_key = NULL, banner_key = NULL, bio = NULL, links = '[]'::jsonb, disabled = true
WHERE id = $1;

-- name: SetUserAdmin :exec
UPDATE users SET is_admin = $2 WHERE id = $1;

-- name: IsIdentifierBanned :one
SELECT EXISTS (
    SELECT 1 FROM banned_identifiers
     WHERE (kind = 'email' AND value = LOWER($1))
        OR (kind = 'username' AND value = LOWER($2))
);

-- name: CreateBannedIdentifier :exec
INSERT INTO banned_identifiers (kind, value, reason, banned_by)
VALUES ($1, LOWER($2), $3, $4)
ON CONFLICT (kind, value) DO NOTHING;

-- name: UpsertEmailVerification :exec
INSERT INTO email_verifications (user_id, code_hash, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET code_hash = $2, expires_at = $3, created_at = NOW();

-- name: GetEmailVerification :one
SELECT code_hash, expires_at FROM email_verifications WHERE user_id = $1;

-- name: MarkEmailVerified :exec
UPDATE users SET email_verified = true WHERE id = $1;

-- name: DeleteEmailVerification :exec
DELETE FROM email_verifications WHERE user_id = $1;
