-- +goose Up
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
