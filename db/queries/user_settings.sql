-- name: GetUserSettings :one
SELECT user_id, timezone, created_at, updated_at
FROM user_settings
WHERE user_id = ?;

-- name: UpsertUserTimezone :one
INSERT INTO user_settings(user_id, timezone, created_at, updated_at)
VALUES(?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
    timezone = excluded.timezone,
    updated_at = excluded.updated_at
RETURNING user_id, timezone, created_at, updated_at;
