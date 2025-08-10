-- name: CreateReminder :one
INSERT INTO reminders(user_id, channel_id, guild_id, message, schedule, at_time, next_run, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, user_id, channel_id, guild_id, message, schedule, at_time, next_run, created_at, updated_at;

-- name: ListByUser :many
SELECT id, user_id, channel_id, guild_id, message, schedule, at_time, next_run, created_at, updated_at
FROM reminders
WHERE user_id = ?
ORDER BY next_run ASC;

-- name: ListDue :many
SELECT id, user_id, channel_id, guild_id, message, schedule, at_time, next_run, created_at, updated_at
FROM reminders
WHERE next_run <= ?
ORDER BY next_run ASC
LIMIT ?;

-- name: DeleteOwned :execrows
DELETE FROM reminders WHERE id = ? AND user_id = ?;

-- name: DeleteByID :exec
DELETE FROM reminders WHERE id = ?;

-- name: SetNextRun :exec
UPDATE reminders SET next_run = ?, updated_at = ? WHERE id = ?;


