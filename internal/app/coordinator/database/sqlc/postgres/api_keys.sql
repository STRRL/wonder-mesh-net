-- name: CreateAPIKey :one
INSERT INTO api_keys (id, wonder_net_id, name, key_hash, key_prefix, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = $1;

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys WHERE id = $1;

-- name: ListAPIKeysByWonderNet :many
SELECT * FROM api_keys WHERE wonder_net_id = $1 ORDER BY created_at DESC;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = $1;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1;
