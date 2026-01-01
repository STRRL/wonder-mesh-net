-- +goose Up
CREATE TABLE wonder_nets (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    headscale_user TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    mesh_type TEXT NOT NULL DEFAULT 'tailscale',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_wonder_nets_owner_id ON wonder_nets(owner_id);

CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    wonder_net_id TEXT NOT NULL REFERENCES wonder_nets(id),
    name TEXT NOT NULL DEFAULT '',
    key_hash TEXT NOT NULL UNIQUE,
    key_prefix TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP
);
CREATE INDEX idx_api_keys_wonder_net_id ON api_keys(wonder_net_id);

-- +goose Down
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS wonder_nets;
