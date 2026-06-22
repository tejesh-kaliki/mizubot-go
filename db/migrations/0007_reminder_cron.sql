-- +goose Up
-- +goose StatementBegin
ALTER TABLE reminders ADD COLUMN cron_expr TEXT NOT NULL DEFAULT '';
ALTER TABLE reminders ADD COLUMN once INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reminders ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC';

UPDATE reminders
SET
    once = CASE WHEN schedule = 'once' THEN 1 ELSE 0 END,
    cron_expr = CASE
        WHEN schedule = 'daily' AND at_time IS NOT NULL AND length(at_time) = 5 THEN substr(at_time, 4, 2) || ' ' || substr(at_time, 1, 2) || ' * * *'
        WHEN schedule = 'hourly' THEN strftime('%M', next_run, 'unixepoch') || ' * * * *'
        ELSE ''
    END,
    timezone = 'UTC';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite cannot drop columns in older versions used by this project.
-- Leave cron columns in place on down migration.
-- +goose StatementEnd
