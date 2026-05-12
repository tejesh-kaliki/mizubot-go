-- name: CreateAnimeEntry :one
INSERT INTO user_anime_entry (
    user_id,
    name,
    keywords,
    channel_id,
    created_at,
    updated_at
)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, user_id, name, keywords, channel_id, latest_guid, latest_title, latest_link, latest_published_at, last_notified_at, created_at, updated_at;

-- name: ListAnimeEntriesByUser :many
SELECT id, user_id, name, keywords, channel_id, latest_guid, latest_title, latest_link, latest_published_at, last_notified_at, created_at, updated_at
FROM user_anime_entry
WHERE user_id = ?
ORDER BY name ASC;

-- name: GetAnimeSettingsByUser :one
SELECT user_id, default_channel_id, created_at, updated_at
FROM user_anime_settings
WHERE user_id = ?;

-- name: UpsertAnimeSettings :one
INSERT INTO user_anime_settings (
    user_id,
    default_channel_id,
    created_at,
    updated_at
)
VALUES (?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
    default_channel_id = excluded.default_channel_id,
    updated_at = excluded.updated_at
RETURNING user_id, default_channel_id, created_at, updated_at;

-- name: ListAnimeEntries :many
SELECT id, user_id, name, keywords, channel_id, latest_guid, latest_title, latest_link, latest_published_at, last_notified_at, created_at, updated_at
FROM user_anime_entry
ORDER BY id ASC;

-- name: DeleteAnimeEntryOwned :execrows
DELETE FROM user_anime_entry
WHERE user_id = ? AND name = ?;

-- name: SetAnimeEntryChannel :execrows
UPDATE user_anime_entry
SET channel_id = ?, updated_at = ?
WHERE user_id = ? AND name = ?;

-- name: CreateProcessedRssEntry :exec
INSERT INTO processed_rss_entry (
    guid,
    title,
    link,
    published_at,
    created_at
)
VALUES (?, ?, ?, ?, ?);

-- name: GetProcessedRssEntry :one
SELECT guid, title, link, published_at, created_at
FROM processed_rss_entry
WHERE guid = ?;

-- name: ListProcessedRssEntriesByGUIDs :many
SELECT guid, title, link, published_at, created_at
FROM processed_rss_entry
WHERE guid IN (sqlc.slice('guids'));

-- name: CreateAnimeMatch :exec
INSERT INTO user_anime_match (
    user_anime_entry_id,
    guid,
    title,
    link,
    published_at,
    created_at
)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListRecentAnimeMatchesByEntryIDs :many
SELECT id, user_anime_entry_id, guid, title, link, published_at, created_at
FROM user_anime_match
WHERE user_anime_entry_id IN (sqlc.slice('entry_ids'))
ORDER BY created_at DESC
LIMIT ?;

-- name: ListRecentAnimeMatchesByUser :many
SELECT m.id, m.user_anime_entry_id, m.guid, m.title, m.link, m.published_at, m.created_at
FROM user_anime_match m
JOIN user_anime_entry e ON e.id = m.user_anime_entry_id
WHERE e.user_id = ?
ORDER BY m.created_at DESC
LIMIT ?;

-- name: HasAnimeMatch :one
SELECT EXISTS(
    SELECT 1
    FROM user_anime_match
    WHERE user_anime_entry_id = ? AND guid = ?
);

-- name: UpdateAnimeEntryLatest :exec
UPDATE user_anime_entry
SET latest_guid = ?,
    latest_title = ?,
    latest_link = ?,
    latest_published_at = ?,
    last_notified_at = ?,
    updated_at = ?
WHERE id = ?;
