-- name: CreateTypeSafeClassificationLog :one
INSERT INTO typesafe_classification_logs(
    guild_id,
    channel_id,
    user_id,
    model,
    request_state,
    request_questions,
    response_answers,
    selected_tools,
    input_tokens,
    output_tokens,
    latency_ms,
    status,
    error,
    created_at,
    message_id
)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, guild_id, channel_id, user_id, model, request_state, request_questions, response_answers, selected_tools, input_tokens, output_tokens, latency_ms, status, error, created_at, message_id;

-- name: ListTypeSafeClassificationLogsByGuild :many
SELECT id, guild_id, channel_id, user_id, model, request_state, request_questions, response_answers, selected_tools, input_tokens, output_tokens, latency_ms, status, error, created_at, message_id
FROM typesafe_classification_logs
WHERE guild_id = ?
ORDER BY created_at DESC
LIMIT ?;
