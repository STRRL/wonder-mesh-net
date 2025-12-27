-- name: CreateOIDCIdentity :exec
INSERT INTO oidc_identities (id, user_id, issuer, subject, email, name, picture, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- name: GetOIDCIdentity :one
SELECT * FROM oidc_identities WHERE id = ?;

-- name: GetOIDCIdentityByIssuerSubject :one
SELECT * FROM oidc_identities WHERE issuer = ? AND subject = ?;

-- name: ListOIDCIdentitiesByUser :many
SELECT * FROM oidc_identities WHERE user_id = ? ORDER BY created_at DESC;

-- name: UpdateOIDCIdentity :exec
UPDATE oidc_identities
SET email = ?, name = ?, picture = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteOIDCIdentity :exec
DELETE FROM oidc_identities WHERE id = ?;

-- name: DeleteOIDCIdentitiesByUser :exec
DELETE FROM oidc_identities WHERE user_id = ?;
