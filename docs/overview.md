## MizuBot Overview

High-level architecture and how reminders work.

### Components

- **Discord bot** (`internal/bot`): Handles slash commands, creates/deletes/lists reminders, and sends messages. Uses `github.com/bwmarrin/discordgo`.
- **Scheduler** (`internal/scheduler`): Periodically queries the DB for due reminders and triggers sends; reschedules or deletes as needed.
- **Persistence** (`internal/reminders`, `internal/db`): SQLite storage for reminders and migrations via `goose`.
- **Config** (`internal/config`): YAML config with environment variable overrides; `-config` flag supported.

### Data model (SQLite)

- **reminders**
  - `id` (INTEGER, PK)
  - `user_id` (TEXT)
  - `channel_id` (TEXT)
  - `guild_id` (TEXT, nullable)
  - `message` (TEXT)
  - `schedule` (TEXT: one of `once|hourly|daily`)
  - `at_time` (TEXT, nullable: `RFC3339` for `once`, `HH:MM` for `daily`)
  - `next_run` (INTEGER: unix seconds, UTC)
  - `created_at` / `updated_at` (INTEGER: unix seconds, UTC)

See `db/migrations/0001_init.sql`.

### Scheduling model

- All times are in **UTC**.
- The scheduler runs on a configurable tick (default `10s`). On each tick it loads reminders where `next_run <= now`.
- For each due reminder:
  - Sends the reminder message to the recorded channel
  - Computes the next run time based on `schedule`:
    - **once**: delete after sending
    - **hourly**: `now + 1h`
    - **daily**: next occurrence of `HH:MM` in UTC
  - Updates `next_run` or deletes the row

### Command surface (slash commands)

- `/remind add message:<string> schedule:(once|hourly|daily) at:<RFC3339|HH:MM>`
  - For `once`: `at` must be RFC3339 (UTC), in the future
  - For `daily`: `at` must be `HH:MM` (UTC)
  - For `hourly`: `at` ignored
- `/remind list` — Lists reminders for the invoking user
- `/remind delete id:<number>` — Deletes a reminder by id (owned by the invoking user)

### Error handling and guarantees

- The scheduler is idempotent per tick; if sending fails, the reminder will be retried on a subsequent tick unless deleted.
- No exactly-once guarantees; messages can be missed or duplicated on process crashes around send/update. Acceptable for simple reminder use cases.
- Granularity is limited by the tick interval; reminders may send up to `tick` late.

### Extensibility

- Add schedules (e.g., cron) by extending `Schedule` enum and `NextAfter`.
- Add per-user time zones by storing TZ and converting inputs/outputs (Discord does not expose user time zones).
- Replace SQLite with external DB by swapping the store implementation.


