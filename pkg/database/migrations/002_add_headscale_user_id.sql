-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN headscale_user_id INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN headscale_user_id;
-- +goose StatementEnd
