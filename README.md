## MizuBot (Go) — Discord Reminder Bot

Minimal Discord bot that schedules reminders (once, hourly, daily) and persists them in SQLite.

### Requirements

- Go 1.24+
- Discord bot token with `bot` and `applications.commands` scopes

### Configuration

You can configure via environment variables or a YAML file (env vars override YAML).

YAML example: see `config.example.yaml`.

- Run with YAML:
  ```bash
  go run ./cmd/mizubot -config ./config.yaml
  ```
- Run with env-only:
  ```bash
  export DISCORD_TOKEN='Bot <token>'
  export DATABASE_PATH=./reminders.db
  export TICK_INTERVAL=10s
  export BOT_ENV=test
  export TEST_GUILD_ID='<guild_id>'
  export DRY_RUN=1
  go run ./cmd/mizubot
  ```

When `BOT_ENV=test` and `TEST_GUILD_ID` are set, the `/remind` slash command is registered only in that guild for fast propagation.

### Slash Commands

- `/remind add message:<text> schedule:(once|hourly|daily) at:<RFC3339|HH:MM>`
- `/remind list`
- `/remind delete id:<number>`

### Tests

```bash
go test ./...
```

Includes unit tests for scheduler, store, and config using in-memory SQLite. No network calls are made during tests.

### Database migrations (goose)

- Migrations live in `db/migrations`.
- Manage via entrypoint:
  ```bash
  # migrate up (default DB path)
  go run ./cmd/migrate -action up

  # specify DB and directory
  go run ./cmd/migrate -dsn ./reminders.db -dir ./db/migrations -action status
  ```
The bot automatically runs `goose up` on start using `./db/migrations`.


