package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mizubot-go/internal/bot"
	"mizubot-go/internal/config"
	"mizubot-go/internal/db"
	"mizubot-go/internal/reminders"
	"mizubot-go/internal/scheduler"
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

	discordBot, err := bot.New(cfg.DiscordToken, store)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched := scheduler.New(store, discordBot.SendReminderMessage, cfg.TickInterval)
	sched.Start(ctx)

	log.Printf("MizuBot is running. Tick: %s", cfg.TickInterval.String())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")

	// Give scheduler a moment to finish current cycle
	time.Sleep(500 * time.Millisecond)
}
