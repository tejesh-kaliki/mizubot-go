## Testing

### Unit tests

- Run: `go test ./...`
- Use in-memory SQLite in tests; no network calls.
- Covered areas:
  - Reminder store: create/list/delete
  - Scheduling: `NextAfter` for once/hourly/daily
  - Scheduler: sends due reminders and reschedules

### Manual testing with a test bot

- Recommended config:
  - `env: test`
  - `test_guild_id: <guild_id>`
  - `tick_interval: 2s`
  - `dry_run: true` (flip off to actually send)
- Start: `go run ./cmd/mizubot -config ./config.yaml`
- Add a reminder: `/remind add message:"ping" schedule:hourly`
- Observe logs; in dry-run it logs the send instead of posting.

### Notes

- Discord does not provide user time zones. All times are UTC.
- For precise timing beyond `tick_interval`, lower the tick at the cost of more DB scans.

### Prod vs Test isolation

- The database path determines isolation. There is no automatic split between prod and test.
- Use separate tokens and separate DB files for prod and test to avoid interference.
- Do not run two bot instances against the same DB file; both will scan and may send duplicates.

Recommended config files:

```yaml
# config.prod.yaml
discord_token: "Bot <prod_token>"
database_path: "./reminders.prod.db"
tick_interval: "10s"
env: "prod"
dry_run: false
```

```yaml
# config.test.yaml
discord_token: "Bot <test_token>"
database_path: "./reminders.test.db"
tick_interval: "2s"
env: "test"
test_guild_id: "<guild_id>"
dry_run: true
```

Run commands:

```bash
# prod
go run ./cmd/mizubot -config ./config.prod.yaml

# test
go run ./cmd/mizubot -config ./config.test.yaml
```


