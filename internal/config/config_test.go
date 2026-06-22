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
  public_feed_base_url: "https://feeds.example.com"
  bucket: "bucket"
  prefix: "feeds"
aws:
  s3_access_key: "key"
  s3_secret_key: "secret"
  s3_region: "us-east-1"
ollama:
  base_url: "http://ollama.local:11434"
  model: "mistral"
  timeout: "5s"
env: "test"
test_guild_id: "G"
dry_run: false
guild_instructions:
  "G":
    "Server rule"
`

func TestLoadFromFileAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DISCORD_TOKEN_TEST", "Bot B")
	t.Setenv("DRY_RUN", "1")
	t.Setenv("OLLAMA_MODEL", "llama3.2")

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
	if cfg.AnimePublicFeedBaseURL != "https://feeds.example.com" {
		t.Fatalf("anime public feed base url: %s", cfg.AnimePublicFeedBaseURL)
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
	if cfg.OllamaBaseURL != "http://ollama.local:11434" {
		t.Fatalf("ollama base url: %s", cfg.OllamaBaseURL)
	}
	if cfg.OllamaModel != "llama3.2" {
		t.Fatalf("ollama model override failed: %s", cfg.OllamaModel)
	}
	if cfg.OllamaTimeout.String() != "5s" {
		t.Fatalf("ollama timeout: %s", cfg.OllamaTimeout)
	}
	if cfg.GuildInstructions["G"] != "Server rule" {
		t.Fatalf("guild instruction mismatch: %#v", cfg.GuildInstructions)
	}
}
