-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS guild_flags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    guidance TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(guild_id, name)
);

CREATE INDEX IF NOT EXISTS idx_guild_flags_guild
ON guild_flags(guild_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_guild_flags_guild;
DROP TABLE IF EXISTS guild_flags;
-- +goose StatementEnd
