-- name: CreateServiceAccount :exec
INSERT INTO service_accounts (id, wonder_net_id, keycloak_client_id, name, created_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP);

-- name: GetServiceAccountByClientID :one
SELECT * FROM service_accounts WHERE keycloak_client_id = ?;

-- name: ListServiceAccountsByWonderNet :many
SELECT * FROM service_accounts WHERE wonder_net_id = ? ORDER BY created_at DESC;

-- name: DeleteServiceAccount :exec
DELETE FROM service_accounts WHERE keycloak_client_id = ?;

-- name: DeleteServiceAccountByID :exec
DELETE FROM service_accounts WHERE id = ?;
