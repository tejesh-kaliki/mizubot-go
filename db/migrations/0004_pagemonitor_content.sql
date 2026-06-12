-- +goose Up
-- +goose StatementBegin
ALTER TABLE page_monitors ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE page_monitors ADD COLUMN last_content TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE page_monitors_new (
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
INSERT INTO page_monitors_new SELECT id, user_id, channel_id, guild_id, url, label, last_status, check_interval, next_check, created_at, updated_at FROM page_monitors;
DROP TABLE page_monitors;
ALTER TABLE page_monitors_new RENAME TO page_monitors;
-- +goose StatementEnd
