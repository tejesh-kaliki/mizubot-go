-- name: CreatePageMonitor :one
INSERT INTO page_monitors(user_id, channel_id, guild_id, url, label, selector, last_status, content_hash, last_content, check_interval, next_check, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, user_id, channel_id, guild_id, url, label, selector, last_status, content_hash, last_content, check_interval, next_check, created_at, updated_at;

-- name: ListPageMonitorsByUser :many
SELECT id, user_id, channel_id, guild_id, url, label, selector, last_status, content_hash, last_content, check_interval, next_check, created_at, updated_at
FROM page_monitors
WHERE user_id = ?
ORDER BY created_at ASC;

-- name: ListDuePageMonitors :many
SELECT id, user_id, channel_id, guild_id, url, label, selector, last_status, content_hash, last_content, check_interval, next_check, created_at, updated_at
FROM page_monitors
WHERE next_check <= ?
ORDER BY next_check ASC
LIMIT ?;

-- name: UpdatePageMonitorContent :exec
UPDATE page_monitors SET last_status = ?, content_hash = ?, last_content = ?, next_check = ?, updated_at = ? WHERE id = ?;

-- name: DeletePageMonitor :execrows
DELETE FROM page_monitors WHERE id = ? AND user_id = ?;
