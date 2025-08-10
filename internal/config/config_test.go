package config

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleYAML = `
discord_token: "Bot A"
database_path: "./db.sqlite"
tick_interval: "2s"
env: "test"
test_guild_id: "G"
dry_run: false
`

func TestLoadFromFileAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DISCORD_TOKEN_TEST", "Bot B")
	t.Setenv("DRY_RUN", "1")

	cfg, err := LoadFromFile(p)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.DiscordToken != "Bot B" {
		t.Fatalf("token override failed: %s", cfg.DiscordToken)
	}
	if cfg.DatabasePath != "./db.sqlite" {
		t.Fatalf("db: %s", cfg.DatabasePath)
	}
	if cfg.TickInterval.String() != "2s" {
		t.Fatalf("tick: %s", cfg.TickInterval)
	}
	if cfg.Env != "test" || cfg.TestGuildID != "G" {
		t.Fatalf("env/testguild: %s/%s", cfg.Env, cfg.TestGuildID)
	}
	if !cfg.DryRun {
		t.Fatalf("dry_run override failed")
	}
}
