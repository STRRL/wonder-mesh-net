-- +goose Up
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE oidc_identities (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    picture TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_oidc_identities_issuer_subject ON oidc_identities(issuer, subject);
CREATE INDEX idx_oidc_identities_user_id ON oidc_identities(user_id);

CREATE TABLE realms (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    headscale_user TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_realms_owner_id ON realms(owner_id);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    realm_id TEXT NOT NULL,
    name TEXT NOT NULL,
    api_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP
);
CREATE INDEX idx_api_keys_realm_id ON api_keys(realm_id);

CREATE TABLE device_requests (
    device_code TEXT PRIMARY KEY,
    user_code TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    realm_id TEXT NOT NULL DEFAULT '',
    headscale_user TEXT NOT NULL DEFAULT '',
    authkey TEXT NOT NULL DEFAULT '',
    headscale_url TEXT NOT NULL DEFAULT '',
    coordinator_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_device_requests_user_code ON device_requests(user_code);
CREATE INDEX idx_device_requests_expires_at ON device_requests(expires_at);

CREATE TABLE auth_states (
    state TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    redirect_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_auth_states_expires_at ON auth_states(expires_at);

-- +goose Down
DROP TABLE IF EXISTS auth_states;
DROP TABLE IF EXISTS device_requests;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS realms;
DROP TABLE IF EXISTS oidc_identities;
DROP TABLE IF EXISTS users;
