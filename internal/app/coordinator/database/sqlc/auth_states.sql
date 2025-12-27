-- name: CreateAuthState :exec
INSERT INTO auth_states (state, provider, redirect_url, created_at, expires_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?);

-- name: GetAuthState :one
SELECT * FROM auth_states WHERE state = ?;

-- name: DeleteAuthState :exec
DELETE FROM auth_states WHERE state = ?;

-- name: DeleteExpiredAuthStates :exec
DELETE FROM auth_states WHERE expires_at < CURRENT_TIMESTAMP;
