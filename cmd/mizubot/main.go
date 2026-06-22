package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mizubot-go/internal/animefeed"
	"mizubot-go/internal/bot"
	"mizubot-go/internal/config"
	"mizubot-go/internal/db"
	"mizubot-go/internal/llm"
	llmtools "mizubot-go/internal/llm/tools"
	"mizubot-go/internal/pagemonitor"
	"mizubot-go/internal/reminders"
	"mizubot-go/internal/scheduler"
	"mizubot-go/internal/usersettings"
)

func main() {
	cfgPath := flag.String("config", "", "path to YAML config")
	flag.Parse()

	var cfg config.Config
	var err error
	if *cfgPath != "" {
		cfg, err = config.LoadFromFile(*cfgPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("db open error: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database, "./db/migrations"); err != nil {
		log.Fatalf("db migrate error: %v", err)
	}

	store := reminders.NewStore(database)
	reminderService := reminders.NewService(store)
	userSettingsStore := usersettings.NewStore(database)
	userSettingsService := usersettings.NewService(userSettingsStore)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var publisher animefeed.Publisher
	if cfg.S3AccessKey != "" && cfg.S3SecretKey != "" && cfg.S3Bucket != "" && cfg.S3Region != "" {
		s3Publisher, err := animefeed.NewS3Publisher(ctx, animefeed.S3PublisherConfig{
			AccessKey:         cfg.S3AccessKey,
			SecretKey:         cfg.S3SecretKey,
			Bucket:            cfg.S3Bucket,
			Region:            cfg.S3Region,
			Prefix:            cfg.S3Prefix,
			PublicFeedBaseURL: cfg.AnimePublicFeedBaseURL,
		})
		if err != nil {
			log.Fatalf("s3 publisher init error: %v", err)
		}
		publisher = s3Publisher
	}

	animeService := animefeed.NewService(database, publisher, cfg.AnimeFeedURL)

	monitorStore := pagemonitor.NewStore(database)
	monitorService := pagemonitor.NewService(monitorStore)

	llmService := llm.NewService(llm.NewOllamaClient(llm.OllamaConfig{
		BaseURL: cfg.OllamaBaseURL,
		Model:   cfg.OllamaModel,
		Timeout: cfg.OllamaTimeout,
	}), append(
		llmtools.NewReminderTools(reminderService, userSettingsService),
		llmtools.NewUserSettingsTools(userSettingsService)...,
	)...)

	discordBot, err := bot.New(cfg.DiscordToken, store, animeService, monitorService, llmService, userSettingsService)
	if err != nil {
		log.Fatalf("discord init error: %v", err)
	}
	// Enable dry-run if requested (no actual sends)
	discordBot.SetDryRun(cfg.DryRun)

	if err := discordBot.Open(); err != nil {
		log.Fatalf("discord open error: %v", err)
	}
	defer discordBot.Close()

	// Register commands after session is opened (application ID is available)
	if cfg.Env == "test" && cfg.TestGuildID != "" {
		if err := discordBot.RegisterCommandsForGuild(cfg.TestGuildID); err != nil {
			log.Printf("guild command register error: %v", err)
		}
	} else {
		if err := discordBot.RegisterCommandsGlobal(); err != nil {
			log.Printf("global command register error: %v", err)
		}
	}

	sched := scheduler.New(store, discordBot.SendReminder, cfg.TickInterval)
	sched.Start(ctx)

	animePoller := animefeed.NewPoller(animeService, discordBot, cfg.AnimePollInterval)
	animePoller.Start(ctx)

	monitorPoller := pagemonitor.NewPoller(monitorService, discordBot, cfg.TickInterval)
	monitorPoller.Start(ctx)

	log.Printf("MizuBot is running. Reminder tick: %s. Anime poll: %s", cfg.TickInterval.String(), cfg.AnimePollInterval.String())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")

	// Give scheduler a moment to finish current cycle
	time.Sleep(500 * time.Millisecond)
}
