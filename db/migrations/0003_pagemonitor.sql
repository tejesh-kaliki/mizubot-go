-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS page_monitors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    guild_id TEXT,
    url TEXT NOT NULL,
    label TEXT NOT NULL,
    last_status TEXT NOT NULL DEFAULT 'unknown',
    check_interval INTEGER NOT NULL DEFAULT 300,
    next_check INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_page_monitors_user_id ON page_monitors(user_id);
CREATE INDEX IF NOT EXISTS idx_page_monitors_next_check ON page_monitors(next_check);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_page_monitors_next_check;
DROP INDEX IF EXISTS idx_page_monitors_user_id;
DROP TABLE IF EXISTS page_monitors;
-- +goose StatementEnd
