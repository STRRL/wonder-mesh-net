-- +goose Up
-- +goose StatementBegin

-- Device authorization flow requests (OAuth 2.0 Device Authorization Grant)
CREATE TABLE device_requests (
    device_code TEXT PRIMARY KEY,
    user_code TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    headscale_user TEXT,
    user_id TEXT,
    authkey TEXT,
    headscale_url TEXT,
    coordinator_url TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_device_requests_user_code ON device_requests(user_code);
CREATE INDEX idx_device_requests_expires_at ON device_requests(expires_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS device_requests;
-- +goose StatementEnd
