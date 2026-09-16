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
  export LLM_BASE_URL=http://localhost:8080/v1
  export LLM_MODEL=llama3.2
  export LLM_API_KEY=""
  export LLM_TIMEOUT=60s
  export BOT_ENV=test
  export TEST_GUILD_ID='<guild_id>'
  export DRY_RUN=1
  go run ./cmd/mizubot
  ```

When `BOT_ENV=test` and `TEST_GUILD_ID` are set, the `/remind` slash command is registered only in that guild for fast propagation.

When the bot is mentioned in Discord, it sends the current message to the configured LLM and replies to that message.

The LLM client speaks the OpenAI chat completions protocol (via the official
[`openai-go`](https://github.com/openai/openai-go) SDK), so `LLM_BASE_URL` should
point at any OpenAI-compatible endpoint — typically a local
[Bifrost](https://getbifrost.ai) gateway, which defaults to port `8080` and can
itself route to Ollama, OpenAI, or other providers. `/v1` is appended
automatically if you don't include it. `LLM_API_KEY` is optional and only
needed if your gateway requires one.

For Docker Compose, a local gateway should be reached through the host gateway. The production compose file sets:

```yaml
LLM_BASE_URL: "http://host.docker.internal:8080/v1"
```

### Slash Commands

- `/remind add message:<text> schedule:(once|hourly|daily) at:<10m|2h|3d|RFC3339|HH:MM|:MM>`
- `/remind list`
- `/remind delete id:<number>`
- `/edit-prompt view` — Show this server's custom system prompt
- `/edit-prompt edit` — Open a text editor modal to change it
- `/edit-prompt reset` — Remove it and fall back to the default personality

`/edit-prompt` is limited to members with Administrator, Manage Server, Manage
Messages, Moderate Members, Kick Members, or Ban Members, plus the user ID in
`owner_discord_id` (env `OWNER_DISCORD_ID`). All its replies are ephemeral.

For one-time reminders, `at` accepts relative durations like `10m`, `2h`, or `3d`. Daily reminders use `HH:MM` UTC, and hourly reminders can use `:MM` for a specific minute each hour.

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
