-- name: CreateAPIKey :exec
INSERT INTO api_keys (id, user_id, name, api_key, scopes, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetAPIKeyByKey :one
SELECT * FROM api_keys WHERE api_key = ?;

-- name: GetAPIKey :one
SELECT * FROM api_keys WHERE id = ?;

-- name: ListAPIKeysByUser :many
SELECT * FROM api_keys WHERE user_id = ? ORDER BY created_at DESC;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = ? WHERE id = ?;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = ?;

-- name: DeleteAPIKeyByUser :execresult
DELETE FROM api_keys WHERE id = ? AND user_id = ?;
