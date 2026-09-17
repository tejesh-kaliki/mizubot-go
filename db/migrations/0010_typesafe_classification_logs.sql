-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS typesafe_classification_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id TEXT,
    channel_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    model TEXT NOT NULL,
    request_state TEXT NOT NULL,
    request_questions TEXT NOT NULL,
    response_answers TEXT NOT NULL DEFAULT '',
    selected_tools TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_typesafe_classification_logs_guild_created
ON typesafe_classification_logs(guild_id, created_at);

CREATE INDEX IF NOT EXISTS idx_typesafe_classification_logs_user_created
ON typesafe_classification_logs(user_id, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_typesafe_classification_logs_user_created;
DROP INDEX IF EXISTS idx_typesafe_classification_logs_guild_created;
DROP TABLE IF EXISTS typesafe_classification_logs;
-- +goose StatementEnd
