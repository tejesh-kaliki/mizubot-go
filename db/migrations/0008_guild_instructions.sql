-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS guild_instructions (
    guild_id TEXT NOT NULL PRIMARY KEY,
    instructions TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS guild_instructions;
-- +goose StatementEnd
