package db

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func Migrate(database *sql.DB, dir string) error {
	goose.SetDialect("sqlite3")
	goose.SetTableName("goose_db_version")
	if err := goose.Up(database, dir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
