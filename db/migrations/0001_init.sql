-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    guild_id TEXT,
    message TEXT NOT NULL,
    schedule TEXT NOT NULL,
    at_time TEXT,
    next_run INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_reminders_user_next ON reminders(user_id, next_run);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_reminders_user_next;
DROP TABLE IF EXISTS reminders;
-- +goose StatementEnd


