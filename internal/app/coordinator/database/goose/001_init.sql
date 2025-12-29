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

-- +goose Down
DROP TABLE IF EXISTS service_accounts;
DROP TABLE IF EXISTS wonder_nets;
