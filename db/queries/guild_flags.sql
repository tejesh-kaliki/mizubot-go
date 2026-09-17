-- name: CreateGuildFlag :one
INSERT INTO guild_flags(guild_id, name, description, guidance, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?)
RETURNING id, guild_id, name, description, guidance, created_at, updated_at;

-- name: ListGuildFlagsByGuild :many
SELECT id, guild_id, name, description, guidance, created_at, updated_at
FROM guild_flags
WHERE guild_id = ?
ORDER BY id;

-- name: UpdateGuildFlag :one
UPDATE guild_flags
SET description = ?, guidance = ?, updated_at = ?
WHERE id = ? AND guild_id = ?
RETURNING id, guild_id, name, description, guidance, created_at, updated_at;

-- name: DeleteGuildFlag :execrows
DELETE FROM guild_flags
WHERE id = ? AND guild_id = ?;
