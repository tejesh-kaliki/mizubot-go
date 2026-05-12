-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS processed_rss_entry (
    guid TEXT NOT NULL PRIMARY KEY,
    title TEXT NOT NULL,
    link TEXT NOT NULL,
    published_at INTEGER,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_anime_entry (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    keywords TEXT NOT NULL CHECK (json_valid(keywords)),
    channel_id TEXT,
    latest_guid TEXT,
    latest_title TEXT,
    latest_link TEXT,
    latest_published_at INTEGER,
    last_notified_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_anime_entry_user_name ON user_anime_entry(user_id, name);
CREATE INDEX IF NOT EXISTS idx_user_anime_entry_user_id ON user_anime_entry(user_id);

CREATE TABLE IF NOT EXISTS user_anime_settings (
    user_id TEXT NOT NULL PRIMARY KEY,
    default_channel_id TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_anime_match (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_anime_entry_id INTEGER NOT NULL,
    guid TEXT NOT NULL,
    title TEXT NOT NULL,
    link TEXT NOT NULL,
    published_at INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (user_anime_entry_id) REFERENCES user_anime_entry(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_anime_match_entry_guid ON user_anime_match(user_anime_entry_id, guid);
CREATE INDEX IF NOT EXISTS idx_user_anime_match_entry_created ON user_anime_match(user_anime_entry_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_user_anime_match_entry_created;
DROP INDEX IF EXISTS idx_user_anime_match_entry_guid;
DROP TABLE IF EXISTS user_anime_match;
DROP TABLE IF EXISTS user_anime_settings;
DROP INDEX IF EXISTS idx_user_anime_entry_user_id;
DROP INDEX IF EXISTS idx_user_anime_entry_user_name;
DROP TABLE IF EXISTS user_anime_entry;
DROP TABLE IF EXISTS processed_rss_entry;
-- +goose StatementEnd
