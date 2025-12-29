-- +goose Up
CREATE TABLE wonder_nets (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    headscale_user TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_wonder_nets_owner_id ON wonder_nets(owner_id);

CREATE TABLE service_accounts (
    id TEXT PRIMARY KEY,
    wonder_net_id TEXT NOT NULL REFERENCES wonder_nets(id),
    keycloak_client_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_service_accounts_wonder_net_id ON service_accounts(wonder_net_id);
CREATE INDEX idx_service_accounts_keycloak_client_id ON service_accounts(keycloak_client_id);

CREATE TABLE device_requests (
    device_code TEXT PRIMARY KEY,
    user_code TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    wonder_net_id TEXT NOT NULL DEFAULT '',
    headscale_user TEXT NOT NULL DEFAULT '',
    authkey TEXT NOT NULL DEFAULT '',
    headscale_url TEXT NOT NULL DEFAULT '',
    coordinator_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_device_requests_user_code ON device_requests(user_code);
CREATE INDEX idx_device_requests_expires_at ON device_requests(expires_at);

-- +goose Down
DROP TABLE IF EXISTS device_requests;
DROP TABLE IF EXISTS service_accounts;
DROP TABLE IF EXISTS wonder_nets;
