package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
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
	LLMBaseURL             string
	LLMModel               string
	LLMAPIKey              string
	LLMTimeout             time.Duration
	GuildInstructions      map[string]string
	LLMDebugHistory        bool
	OwnerDiscordID         string
	LLMJevAPIKey           string
	LLMJevBaseURL          string
	LLMJevModel            string
	LLMJevTimeout          time.Duration
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
	LLM               llmFileConfig     `yaml:"llm"`
	GuildInstructions map[string]string `yaml:"guild_instructions"`
	LLMDebugHistory   bool              `yaml:"llm_debug_history"`
	OwnerDiscordID    string            `yaml:"owner_discord_id"`
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

type llmFileConfig struct {
	BaseURL    string `yaml:"base_url"`
	Model      string `yaml:"model"`
	APIKey     string `yaml:"api_key"`
	Timeout    string `yaml:"timeout"`
	JevAPIKey  string `yaml:"jev_api_key"`
	JevBaseURL string `yaml:"jev_base_url"`
	JevModel   string `yaml:"jev_model"`
	JevTimeout string `yaml:"jev_timeout"`
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
	LLMBaseURL             string
	LLMModel               string
	LLMAPIKey              string
	LLMTimeout             string
	LLMDebugHistory        string
	OwnerDiscordID         string
	LLMJevAPIKey           string
	LLMJevBaseURL          string
	LLMJevModel            string
	LLMJevTimeout          string
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
		LLMBaseURL:             os.Getenv("LLM_BASE_URL"),
		LLMModel:               os.Getenv("LLM_MODEL"),
		LLMAPIKey:              os.Getenv("LLM_API_KEY"),
		LLMTimeout:             os.Getenv("LLM_TIMEOUT"),
		LLMDebugHistory:        os.Getenv("LLM_DEBUG_HISTORY"),
		OwnerDiscordID:         os.Getenv("OWNER_DISCORD_ID"),
		LLMJevAPIKey:           os.Getenv("LLM_JEV_API_KEY"),
		LLMJevBaseURL:          os.Getenv("LLM_JEV_BASE_URL"),
		LLMJevModel:            os.Getenv("LLM_JEV_MODEL"),
		LLMJevTimeout:          os.Getenv("LLM_JEV_TIMEOUT"),
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

	llmDebugHistory := f.LLMDebugHistory
	if e.LLMDebugHistory == "1" || e.LLMDebugHistory == "true" || e.LLMDebugHistory == "TRUE" {
		llmDebugHistory = true
	}

	llmTimeoutStr := fallback(e.LLMTimeout, f.LLM.Timeout, "60s")
	llmTimeout := time.Minute
	if d, err := time.ParseDuration(llmTimeoutStr); err == nil {
		llmTimeout = d
	}

	testGuild := fallback(e.TestGuildID, f.TestGuildID, "")

	jevTimeoutStr := fallback(e.LLMJevTimeout, f.LLM.JevTimeout, "15s")
	jevTimeout := 15 * time.Second
	if d, err := time.ParseDuration(jevTimeoutStr); err == nil {
		jevTimeout = d
	}

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
		LLMBaseURL:             fallback(e.LLMBaseURL, f.LLM.BaseURL, "http://localhost:8080/v1"),
		LLMModel:               fallback(e.LLMModel, f.LLM.Model, "llama3.2"),
		LLMAPIKey:              fallback(e.LLMAPIKey, f.LLM.APIKey, ""),
		LLMTimeout:             llmTimeout,
		GuildInstructions:      f.GuildInstructions,
		LLMDebugHistory:        llmDebugHistory,
		OwnerDiscordID:         strings.TrimSpace(fallback(e.OwnerDiscordID, f.OwnerDiscordID, "")),
		LLMJevAPIKey:           fallback(e.LLMJevAPIKey, f.LLM.JevAPIKey, ""),
		LLMJevBaseURL:          fallback(e.LLMJevBaseURL, f.LLM.JevBaseURL, ""),
		LLMJevModel:            fallback(e.LLMJevModel, f.LLM.JevModel, ""),
		LLMJevTimeout:          jevTimeout,
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
