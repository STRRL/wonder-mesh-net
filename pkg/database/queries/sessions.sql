-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, issuer, subject, created_at, expires_at, last_used_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT * FROM sessions WHERE id = ?;

-- name: GetSessionByUserID :many
SELECT * FROM sessions WHERE user_id = ? ORDER BY created_at DESC;

-- name: UpdateSessionLastUsed :exec
UPDATE sessions SET last_used_at = ? WHERE id = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = ?;
