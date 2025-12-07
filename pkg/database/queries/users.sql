-- name: CreateUser :exec
INSERT INTO users (id, headscale_user, headscale_user_id, issuer, subject, email, name, picture, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByHeadscaleUser :one
SELECT * FROM users WHERE headscale_user = ?;

-- name: GetUserByIssuerSubject :one
SELECT * FROM users WHERE issuer = ? AND subject = ?;

-- name: UpdateUser :exec
UPDATE users
SET email = ?, name = ?, picture = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;
