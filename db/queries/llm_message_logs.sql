-- name: CreateLLMMessageLog :one
INSERT INTO llm_message_logs(
    guild_id,
    channel_id,
    user_id,
    message_id,
    prompt_tokens,
    completion_tokens,
    total_tokens,
    llm_turns,
    tool_calls,
    latency_ms,
    status,
    error,
    created_at
)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, guild_id, channel_id, user_id, message_id, prompt_tokens, completion_tokens, total_tokens, llm_turns, tool_calls, latency_ms, status, error, created_at;

-- name: ListLLMMessageLogsByGuild :many
SELECT id, guild_id, channel_id, user_id, message_id, prompt_tokens, completion_tokens, total_tokens, llm_turns, tool_calls, latency_ms, status, error, created_at
FROM llm_message_logs
WHERE guild_id = ?
ORDER BY created_at DESC
LIMIT ?;
