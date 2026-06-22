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
	DiscordToken           string
	DatabasePath           string
	TickInterval           time.Duration
	AnimePollInterval      time.Duration
	AnimeFeedURL           string
	AnimePublicFeedBaseURL string
	S3AccessKey            string
	S3SecretKey            string
	S3Bucket               string
	S3Region               string
	S3Prefix               string
	Env                    string // "prod" or "test"
	TestGuildID            string
	DryRun                 bool
	OllamaBaseURL          string
	OllamaModel            string
	OllamaTimeout          time.Duration
	GuildInstructions      map[string]string
}

type fileConfig struct {
	DiscordToken      string            `yaml:"discord_token"`
	DatabasePath      string            `yaml:"database_path"`
	TickInterval      string            `yaml:"tick_interval"`
	Anime             animeFileConfig   `yaml:"anime"`
	AWS               awsFileConfig     `yaml:"aws"`
	Env               string            `yaml:"env"`
	TestGuildID       string            `yaml:"test_guild_id"`
	DryRun            bool              `yaml:"dry_run"`
	Ollama            ollamaFileConfig  `yaml:"ollama"`
	GuildInstructions map[string]string `yaml:"guild_instructions"`
}

type animeFileConfig struct {
	PollInterval      string `yaml:"poll_interval"`
	FeedURL           string `yaml:"feed_url"`
	PublicFeedBaseURL string `yaml:"public_feed_base_url"`
	Bucket            string `yaml:"bucket"`
	Prefix            string `yaml:"prefix"`
}

type awsFileConfig struct {
	S3AccessKey string `yaml:"s3_access_key"`
	S3SecretKey string `yaml:"s3_secret_key"`
	S3Region    string `yaml:"s3_region"`
}

type ollamaFileConfig struct {
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
	Timeout string `yaml:"timeout"`
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
	Env                    string
	Token                  string
	TokenTest              string
	DBPath                 string
	Tick                   string
	AnimePoll              string
	AnimeFeedURL           string
	AnimePublicFeedBaseURL string
	S3AccessKey            string
	S3SecretKey            string
	S3Bucket               string
	S3Region               string
	S3Prefix               string
	DryRun                 string
	TestGuildID            string
	OllamaBaseURL          string
	OllamaModel            string
	OllamaTimeout          string
}

func osEnv() envVals {
	return envVals{
		Env:                    os.Getenv("BOT_ENV"),
		Token:                  os.Getenv("DISCORD_TOKEN"),
		TokenTest:              os.Getenv("DISCORD_TOKEN_TEST"),
		DBPath:                 os.Getenv("DATABASE_PATH"),
		Tick:                   os.Getenv("TICK_INTERVAL"),
		AnimePoll:              os.Getenv("ANIME_POLL_INTERVAL"),
		AnimeFeedURL:           os.Getenv("ANIME_FEED_URL"),
		AnimePublicFeedBaseURL: os.Getenv("ANIME_PUBLIC_FEED_BASE_URL"),
		S3AccessKey:            os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:            os.Getenv("S3_SECRET_KEY"),
		S3Bucket:               os.Getenv("S3_BUCKET"),
		S3Region:               os.Getenv("S3_REGION"),
		S3Prefix:               os.Getenv("S3_PREFIX"),
		DryRun:                 os.Getenv("DRY_RUN"),
		TestGuildID:            os.Getenv("TEST_GUILD_ID"),
		OllamaBaseURL:          os.Getenv("OLLAMA_BASE_URL"),
		OllamaModel:            os.Getenv("OLLAMA_MODEL"),
		OllamaTimeout:          os.Getenv("OLLAMA_TIMEOUT"),
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

	animePollStr := fallback(e.AnimePoll, f.Anime.PollInterval, "1m")
	animePoll := time.Minute
	if d, err := time.ParseDuration(animePollStr); err == nil {
		animePoll = d
	}

	dry := f.DryRun
	if e.DryRun == "1" || e.DryRun == "true" || e.DryRun == "TRUE" {
		dry = true
	}

	ollamaTimeoutStr := fallback(e.OllamaTimeout, f.Ollama.Timeout, "60s")
	ollamaTimeout := time.Minute
	if d, err := time.ParseDuration(ollamaTimeoutStr); err == nil {
		ollamaTimeout = d
	}

	testGuild := fallback(e.TestGuildID, f.TestGuildID, "")

	return Config{
		DiscordToken:           token,
		DatabasePath:           dbPath,
		TickInterval:           tick,
		AnimePollInterval:      animePoll,
		AnimeFeedURL:           fallback(e.AnimeFeedURL, f.Anime.FeedURL, ""),
		AnimePublicFeedBaseURL: fallback(e.AnimePublicFeedBaseURL, f.Anime.PublicFeedBaseURL, ""),
		S3AccessKey:            fallback(e.S3AccessKey, f.AWS.S3AccessKey, ""),
		S3SecretKey:            fallback(e.S3SecretKey, f.AWS.S3SecretKey, ""),
		S3Bucket:               fallback(e.S3Bucket, f.Anime.Bucket, ""),
		S3Region:               fallback(e.S3Region, f.AWS.S3Region, ""),
		S3Prefix:               fallback(e.S3Prefix, f.Anime.Prefix, ""),
		Env:                    env,
		DryRun:                 dry,
		TestGuildID:            testGuild,
		OllamaBaseURL:          fallback(e.OllamaBaseURL, f.Ollama.BaseURL, "http://localhost:11434"),
		OllamaModel:            fallback(e.OllamaModel, f.Ollama.Model, "llama3.2"),
		OllamaTimeout:          ollamaTimeout,
		GuildInstructions:      f.GuildInstructions,
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
