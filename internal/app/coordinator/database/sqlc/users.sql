-- name: CreateUser :exec
INSERT INTO users (id, keycloak_sub, display_name, created_at, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByKeycloakSub :one
SELECT * FROM users WHERE keycloak_sub = ?;

-- name: UpdateUser :exec
UPDATE users SET display_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;
