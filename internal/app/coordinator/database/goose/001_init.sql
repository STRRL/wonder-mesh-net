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

-- +goose Down
DROP TABLE IF EXISTS wonder_nets;
