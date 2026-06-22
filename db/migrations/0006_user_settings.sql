-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS user_settings (
    user_id TEXT NOT NULL PRIMARY KEY,
    timezone TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_settings;
-- +goose StatementEnd
