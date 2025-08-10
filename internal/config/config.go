package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DiscordToken string
	DatabasePath string
	TickInterval time.Duration
	Env          string // "prod" or "test"
	TestGuildID  string
	DryRun       bool
}

type fileConfig struct {
	DiscordToken string `yaml:"discord_token"`
	DatabasePath string `yaml:"database_path"`
	TickInterval string `yaml:"tick_interval"`
	Env          string `yaml:"env"`
	TestGuildID  string `yaml:"test_guild_id"`
	DryRun       bool   `yaml:"dry_run"`
}

// Load keeps env-only behavior for backward compatibility
func Load() (Config, error) {
	return fromValues(fileConfig{}, osEnv())
}

// LoadFromFile loads YAML config and applies environment variable overrides.
func LoadFromFile(path string) (Config, error) {
	fc := fileConfig{}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return Config{}, fmt.Errorf("open config: %w", err)
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(b, &fc); err != nil {
			return Config{}, fmt.Errorf("parse yaml: %w", err)
		}
	}
	return fromValues(fc, osEnv())
}

type envVals struct {
	Env         string
	Token       string
	TokenTest   string
	DBPath      string
	Tick        string
	DryRun      string
	TestGuildID string
}

func osEnv() envVals {
	return envVals{
		Env:         os.Getenv("BOT_ENV"),
		Token:       os.Getenv("DISCORD_TOKEN"),
		TokenTest:   os.Getenv("DISCORD_TOKEN_TEST"),
		DBPath:      os.Getenv("DATABASE_PATH"),
		Tick:        os.Getenv("TICK_INTERVAL"),
		DryRun:      os.Getenv("DRY_RUN"),
		TestGuildID: os.Getenv("TEST_GUILD_ID"),
	}
}

func fromValues(f fileConfig, e envVals) (Config, error) {
	env := fallback(e.Env, f.Env, "prod")

	token := f.DiscordToken
	if env == "test" && e.TokenTest != "" {
		token = e.TokenTest
	}
	if e.Token != "" {
		token = e.Token
	}
	if token == "" {
		return Config{}, errors.New("missing token: set discord_token in YAML or DISCORD_TOKEN (or DISCORD_TOKEN_TEST in test env)")
	}

	dbPath := fallback(e.DBPath, f.DatabasePath, "./reminders.db")

	tickStr := fallback(e.Tick, f.TickInterval, "10s")
	tick := 10 * time.Second
	if d, err := time.ParseDuration(tickStr); err == nil {
		tick = d
	}

	dry := f.DryRun
	if e.DryRun == "1" || e.DryRun == "true" || e.DryRun == "TRUE" {
		dry = true
	}

	testGuild := fallback(e.TestGuildID, f.TestGuildID, "")

	return Config{
		DiscordToken: token,
		DatabasePath: dbPath,
		TickInterval: tick,
		Env:          env,
		DryRun:       dry,
		TestGuildID:  testGuild,
	}, nil
}

func fallback(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
