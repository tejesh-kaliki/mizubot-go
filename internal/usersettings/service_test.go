package usersettings

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE user_settings (
		user_id TEXT NOT NULL PRIMARY KEY,
		timezone TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTimezoneSettings(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	service := NewService(NewStore(db))

	timezone, configured, err := service.GetTimezone(context.Background(), "u")
	if err != nil {
		t.Fatalf("GetTimezone: %v", err)
	}
	if configured {
		t.Fatalf("configured = true, want false")
	}
	if timezone != DefaultTimezone {
		t.Fatalf("timezone = %q, want %q", timezone, DefaultTimezone)
	}

	settings, err := service.SetTimezone(context.Background(), "u", "Asia/Kolkata")
	if err != nil {
		t.Fatalf("SetTimezone: %v", err)
	}
	if settings.Timezone != "Asia/Kolkata" {
		t.Fatalf("timezone = %q, want Asia/Kolkata", settings.Timezone)
	}

	timezone, configured, err = service.GetTimezone(context.Background(), "u")
	if err != nil {
		t.Fatalf("GetTimezone after set: %v", err)
	}
	if !configured || timezone != "Asia/Kolkata" {
		t.Fatalf("configured/timezone = %v/%q, want true/Asia/Kolkata", configured, timezone)
	}
}

func TestSetTimezoneResolvesFriendlyTimezone(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	service := NewService(NewStore(db))
	settings, err := service.SetTimezone(context.Background(), "u", "India")
	if err != nil {
		t.Fatalf("SetTimezone India: %v", err)
	}
	if settings.Timezone != "Asia/Kolkata" {
		t.Fatalf("timezone = %q, want Asia/Kolkata", settings.Timezone)
	}

	settings, err = service.SetTimezone(context.Background(), "u", "new york")
	if err != nil {
		t.Fatalf("SetTimezone new york: %v", err)
	}
	if settings.Timezone != "America/New_York" {
		t.Fatalf("timezone = %q, want America/New_York", settings.Timezone)
	}
}

func TestSetTimezoneRejectsInvalidTimezone(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	service := NewService(NewStore(db))
	if _, err := service.SetTimezone(context.Background(), "u", "not a timezone"); err == nil {
		t.Fatalf("expected invalid timezone error")
	}
}
