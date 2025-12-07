-- +goose Up
-- +goose StatementBegin

-- OIDC auth states for OAuth flow (short-lived, ~10 min TTL)
CREATE TABLE auth_states (
    state TEXT PRIMARY KEY,
    nonce TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    provider_name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_auth_states_expires_at ON auth_states(expires_at);

-- User sessions (long-lived)
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- Local user cache (mirrors Headscale users with extra metadata)
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    headscale_user TEXT NOT NULL UNIQUE,
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    email TEXT,
    name TEXT,
    picture TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_users_issuer_subject ON users(issuer, subject);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS auth_states;
-- +goose StatementEnd
