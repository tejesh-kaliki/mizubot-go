-- name: GetGuildInstructions :one
SELECT guild_id, instructions, created_at, updated_at
FROM guild_instructions
WHERE guild_id = ?;

-- name: UpsertGuildInstructions :one
INSERT INTO guild_instructions(guild_id, instructions, created_at, updated_at)
VALUES(?, ?, ?, ?)
ON CONFLICT(guild_id) DO UPDATE SET
    instructions = excluded.instructions,
    updated_at = excluded.updated_at
RETURNING guild_id, instructions, created_at, updated_at;
