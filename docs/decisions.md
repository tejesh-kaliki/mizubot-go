## Key Decisions (ADR-style summary)

- **Language: Go 1.24**
  - Rationale: concurrency model, strong tooling, single static binary.

- **Discord library: `bwmarrin/discordgo`**
  - Rationale: mature, widely used, fits slash commands and gateway needs.

- **Storage: SQLite via `modernc.org/sqlite` (pure Go)**
  - Rationale: zero CGO dependency for portability; adequate for single-process bot.
  - Trade-off: lower throughput than external DB; fine for reminders.

- **Migrations: `pressly/goose`**
  - Rationale: simple SQL migration engine; CLI entrypoint included; auto-run on start.

- **Time: Use UTC everywhere**
  - Rationale: Discord does not expose user time zones; UTC reduces ambiguity.
  - Impact: `daily` schedule uses `HH:MM` in UTC; `once` uses RFC3339 (UTC).

- **Scheduler: in-process ticker scan**
  - Rationale: simple and reliable for small load; avoids external schedulers.
  - Trade-off: delivery granularity equals tick (default 10s); not exactly-once.

- **Commands: Slash commands only**
  - Rationale: structured UX, permissioned per guild, ephemeral responses supported.

- **Config: YAML with env var overrides**
  - Rationale: easy local/dev sharing; env for secrets and overrides; `-config` flag.

- **Dry-run mode**
  - Rationale: safe testing without posting to channels; logs messages instead.

- **Guild-scoped command registration in test**
  - Rationale: faster propagation vs global; controlled via config.

Future considerations:

- Per-user time zone preference and localized input/output.
- Richer schedules (e.g., cron expressions, week-days, intervals).
- Persistence retries/queue and stronger delivery guarantees.
- Sharding or external DB if scale grows.


