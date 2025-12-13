-- +goose Up
-- +goose StatementBegin
CREATE TABLE users_new (
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

INSERT INTO users_new (id, headscale_user, issuer, subject, email, name, picture, created_at, updated_at)
SELECT id, headscale_user, issuer, subject, email, name, picture, created_at, updated_at FROM users;

DROP TABLE users;

ALTER TABLE users_new RENAME TO users;

CREATE UNIQUE INDEX idx_users_issuer_subject ON users(issuer, subject);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN headscale_user_id INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd
