-- name: CreateAPIKey :one
INSERT INTO api_keys (id, wonder_net_id, name, key_hash, key_prefix, expires_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = ?;

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys WHERE id = ?;

-- name: ListAPIKeysByWonderNet :many
SELECT * FROM api_keys WHERE wonder_net_id = ? ORDER BY created_at DESC;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = ?;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?;
