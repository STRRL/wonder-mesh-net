-- name: CreateWonderNet :exec
INSERT INTO wonder_nets (id, owner_id, headscale_user, display_name, mesh_type, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- name: GetWonderNet :one
SELECT * FROM wonder_nets WHERE id = ?;

-- name: GetWonderNetByHeadscaleUser :one
SELECT * FROM wonder_nets WHERE headscale_user = ?;

-- name: ListWonderNetsByOwner :many
SELECT * FROM wonder_nets WHERE owner_id = ? ORDER BY created_at DESC;

-- name: UpdateWonderNet :exec
UPDATE wonder_nets
SET display_name = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteWonderNet :exec
DELETE FROM wonder_nets WHERE id = ?;

-- name: ListWonderNets :many
SELECT * FROM wonder_nets ORDER BY created_at DESC;
