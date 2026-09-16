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
llm:
  base_url: "http://bifrost.local:8080/v1"
  model: "mistral"
  api_key: "test-key"
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
	t.Setenv("LLM_MODEL", "llama3.2")

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
	if cfg.LLMBaseURL != "http://bifrost.local:8080/v1" {
		t.Fatalf("llm base url: %s", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "llama3.2" {
		t.Fatalf("llm model override failed: %s", cfg.LLMModel)
	}
	if cfg.LLMAPIKey != "test-key" {
		t.Fatalf("llm api key: %s", cfg.LLMAPIKey)
	}
	if cfg.LLMTimeout.String() != "5s" {
		t.Fatalf("llm timeout: %s", cfg.LLMTimeout)
	}
	if cfg.GuildInstructions["G"] != "Server rule" {
		t.Fatalf("guild instruction mismatch: %#v", cfg.GuildInstructions)
	}
	if cfg.LLMDebugHistory {
		t.Fatalf("llm_debug_history should default to false")
	}
}

func TestLLMDebugHistoryFromFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML+"\nllm_debug_history: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromFile(p)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if !cfg.LLMDebugHistory {
		t.Fatalf("llm_debug_history from file should be true")
	}

	t.Setenv("LLM_DEBUG_HISTORY", "1")
	cfg, err = LoadFromFile(p)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if !cfg.LLMDebugHistory {
		t.Fatalf("LLM_DEBUG_HISTORY=1 env override should enable debug history")
	}
}

func TestOwnerDiscordIDFromFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML+"\nowner_discord_id: \"  111  \"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromFile(p)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.OwnerDiscordID != "111" {
		t.Fatalf("owner id = %q, want the trimmed 111", cfg.OwnerDiscordID)
	}

	t.Setenv("OWNER_DISCORD_ID", "222")
	cfg, err = LoadFromFile(p)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.OwnerDiscordID != "222" {
		t.Fatalf("owner id = %q, want the env override 222", cfg.OwnerDiscordID)
	}
}

func TestOwnerDiscordIDDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromFile(p)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.OwnerDiscordID != "" {
		t.Fatalf("owner id = %q, want empty by default", cfg.OwnerDiscordID)
	}
}
