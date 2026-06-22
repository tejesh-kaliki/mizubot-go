-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS llm_message_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT,
    channel_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    llm_turns INTEGER NOT NULL DEFAULT 0,
    tool_calls INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_llm_message_logs_guild_created
ON llm_message_logs(guild_id, created_at);

CREATE INDEX IF NOT EXISTS idx_llm_message_logs_user_created
ON llm_message_logs(user_id, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_llm_message_logs_user_created;
DROP INDEX IF EXISTS idx_llm_message_logs_guild_created;
DROP TABLE IF EXISTS llm_message_logs;
-- +goose StatementEnd
