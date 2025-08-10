package main

import (
	"database/sql"
	"flag"
	"log"
	"os"

	"github.com/pressly/goose/v3"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

type fileConfig struct {
	DatabasePath string `yaml:"database_path"`
}

func main() {
	cfgPath := flag.String("config", "", "path to YAML config (uses database_path)")
	dsn := flag.String("dsn", "", "sqlite database path (overrides -config if set)")
	dir := flag.String("dir", "./db/migrations", "migrations directory")
	action := flag.String("action", "up", "goose action: up|down|status|redo|reset|up-by-one|down-to <version> etc.")
	to := flag.Int64("to", 0, "target version for certain actions")
	flag.Parse()

	resolvedDSN := ""
	if *cfgPath != "" {
		b, err := os.ReadFile(*cfgPath)
		if err != nil {
			log.Fatalf("read config: %v", err)
		}
		var fc fileConfig
		if err := yaml.Unmarshal(b, &fc); err != nil {
			log.Fatalf("parse config yaml: %v", err)
		}
		if fc.DatabasePath != "" {
			resolvedDSN = fc.DatabasePath
		}
	}
	if *dsn != "" {
		resolvedDSN = *dsn
	}
	if resolvedDSN == "" {
		resolvedDSN = os.Getenv("DATABASE_PATH")
	}
	if resolvedDSN == "" {
		resolvedDSN = "./reminders.db"
	}

	db, err := sql.Open("sqlite", resolvedDSN)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	goose.SetDialect("sqlite3")
	goose.SetTableName("goose_db_version")

	switch *action {
	case "up":
		err = goose.Up(db, *dir)
	case "down":
		err = goose.Down(db, *dir)
	case "status":
		err = goose.Status(db, *dir)
	case "redo":
		err = goose.Redo(db, *dir)
	case "reset":
		err = goose.Reset(db, *dir)
	case "up-by-one":
		err = goose.UpByOne(db, *dir)
	case "down-to":
		if *to <= 0 {
			log.Fatal("-to required")
		}
		err = goose.DownTo(db, *dir, *to)
	case "up-to":
		if *to <= 0 {
			log.Fatal("-to required")
		}
		err = goose.UpTo(db, *dir, *to)
	default:
		log.Fatalf("unknown action: %s", *action)
	}
	if err != nil {
		log.Fatal(err)
	}
}
