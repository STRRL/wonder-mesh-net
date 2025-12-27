-- name: CreateRealm :exec
INSERT INTO realms (id, owner_id, headscale_user, display_name, created_at, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- name: GetRealm :one
SELECT * FROM realms WHERE id = ?;

-- name: GetRealmByHeadscaleUser :one
SELECT * FROM realms WHERE headscale_user = ?;

-- name: ListRealmsByOwner :many
SELECT * FROM realms WHERE owner_id = ? ORDER BY created_at DESC;

-- name: UpdateRealm :exec
UPDATE realms
SET display_name = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteRealm :exec
DELETE FROM realms WHERE id = ?;

-- name: ListRealms :many
SELECT * FROM realms ORDER BY created_at DESC;
