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
anime:
  poll_interval: "1m"
  feed_url: "https://example.com/feed.xml"
  bucket: "bucket"
  prefix: "feeds"
aws:
  s3_access_key: "key"
  s3_secret_key: "secret"
  s3_region: "us-east-1"
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
	if cfg.AnimePollInterval.String() != "1m0s" {
		t.Fatalf("anime poll: %s", cfg.AnimePollInterval)
	}
	if cfg.AnimeFeedURL != "https://example.com/feed.xml" {
		t.Fatalf("anime feed url: %s", cfg.AnimeFeedURL)
	}
	if cfg.S3Bucket != "bucket" || cfg.S3Region != "us-east-1" || cfg.S3Prefix != "feeds" {
		t.Fatalf("s3 config mismatch: %#v", cfg)
	}
	if cfg.Env != "test" || cfg.TestGuildID != "G" {
		t.Fatalf("env/testguild: %s/%s", cfg.Env, cfg.TestGuildID)
	}
	if !cfg.DryRun {
		t.Fatalf("dry_run override failed")
	}
}
