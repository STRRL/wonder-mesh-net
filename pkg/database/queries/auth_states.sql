-- name: CreateAuthState :exec
INSERT INTO auth_states (state, nonce, redirect_uri, provider_name, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetAuthState :one
SELECT * FROM auth_states WHERE state = ?;

-- name: DeleteAuthState :exec
DELETE FROM auth_states WHERE state = ?;

-- name: DeleteExpiredAuthStates :exec
DELETE FROM auth_states WHERE expires_at < CURRENT_TIMESTAMP;
