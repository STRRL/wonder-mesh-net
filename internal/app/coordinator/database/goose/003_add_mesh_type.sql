-- +goose Up
-- Add mesh_type column to wonder_nets for multi-backend support
-- Default to 'tailscale' for existing rows since that's the only backend currently supported
ALTER TABLE wonder_nets ADD COLUMN mesh_type TEXT NOT NULL DEFAULT 'tailscale';

-- +goose Down
ALTER TABLE wonder_nets DROP COLUMN mesh_type;
