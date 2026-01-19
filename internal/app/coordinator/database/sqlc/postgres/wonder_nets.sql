-- name: CreateWonderNet :exec
INSERT INTO wonder_nets (id, owner_id, headscale_user, display_name, mesh_type, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- name: GetWonderNet :one
SELECT * FROM wonder_nets WHERE id = $1;

-- name: GetWonderNetByHeadscaleUser :one
SELECT * FROM wonder_nets WHERE headscale_user = $1;

-- name: ListWonderNetsByOwner :many
SELECT * FROM wonder_nets WHERE owner_id = $1 ORDER BY created_at DESC;

-- name: UpdateWonderNet :exec
UPDATE wonder_nets
SET display_name = $1, updated_at = CURRENT_TIMESTAMP
WHERE id = $2;

-- name: DeleteWonderNet :exec
DELETE FROM wonder_nets WHERE id = $1;

-- name: ListWonderNets :many
SELECT * FROM wonder_nets ORDER BY created_at DESC;
