## Operations Guide

### Configuration

- Prefer YAML config with env overrides. See `config.example.yaml`.
- Start with YAML: `go run ./cmd/mizubot -config ./config.yaml`
- Start with env-only: export vars then `go run ./cmd/mizubot`.

Key settings:
- **discord_token**: `Bot <token>`
- **database_path**: SQLite file path
- **tick_interval**: e.g., `10s` (affects delivery granularity)
- **env**: `prod|test` (test uses guild-scoped commands if `test_guild_id` set)
- **dry_run**: log instead of sending messages

### Database migrations

- Run on startup automatically against `./db/migrations`.
- Manual control via entrypoint:
  - `go run ./cmd/migrate -action up`
  - `go run ./cmd/migrate -dsn ./reminders.db -dir ./db/migrations -action status`

### Permissions and intents

- Bot requires permissions to send messages in target channels.
- Slash commands require `applications.commands` scope; invite with `bot` + `applications.commands`.

### Deployment

- Single binary; mount a writable directory for the SQLite DB file.
- Back up SQLite file periodically if reminders are important.
- Consider running with `TICK_INTERVAL` >= `5s` to reduce load.
- Use separate config files and DB files for prod vs test environments to avoid cross-talk.
- Never run multiple bot instances on the same DB file; duplicates may occur.

### Observability

- Logs show scheduler actions, send errors, and dry-run messages.
- No metrics out-of-the-box; add Prometheus if needed.

### Failure modes

- Process crashes around send/update can cause duplicate or missed sends.
- If send fails (Discord error), reminder stays due and will retry on the next tick.


