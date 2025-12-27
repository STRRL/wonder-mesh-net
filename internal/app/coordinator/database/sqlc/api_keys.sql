-- name: CreateAPIKey :exec
INSERT INTO api_keys (id, realm_id, name, api_key, created_at, expires_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?);

-- name: GetAPIKeyByKey :one
SELECT * FROM api_keys WHERE api_key = ?;

-- name: GetAPIKey :one
SELECT * FROM api_keys WHERE id = ?;

-- name: ListAPIKeysByRealm :many
SELECT * FROM api_keys WHERE realm_id = ? ORDER BY created_at DESC;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = ?;

-- name: DeleteAPIKeyByRealm :execresult
DELETE FROM api_keys WHERE id = ? AND realm_id = ?;
