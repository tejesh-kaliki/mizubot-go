package reminders

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE reminders (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id TEXT NOT NULL,
        channel_id TEXT NOT NULL,
        guild_id TEXT,
        message TEXT NOT NULL,
        schedule TEXT NOT NULL,
        at_time TEXT,
        next_run INTEGER NOT NULL,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL
    );`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCreateListDeleteReminder(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	now := time.Now()
	r := &Reminder{UserID: "u", ChannelID: "c", Message: "hello", Schedule: ScheduleHourly, NextRun: now.Add(time.Hour)}
	if err := store.Create(context.Background(), r); err != nil {
		t.Fatalf("create: %v", err)
	}
	if r.ID == 0 {
		t.Fatalf("expected id")
	}
	list, err := store.ListByUser(context.Background(), "u")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	ok, err := store.Delete(context.Background(), r.ID, "u")
	if err != nil || !ok {
		t.Fatalf("delete: %v ok=%v", err, ok)
	}
}

func TestNextAfter(t *testing.T) {
	now := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)
	hourly := Reminder{Schedule: ScheduleHourly}
	next, rep, err := NextAfter(hourly, now)
	if err != nil || !rep || !next.Equal(now.Add(time.Hour)) {
		t.Fatalf("hourly: next=%v rep=%v err=%v", next, rep, err)
	}

	daily := Reminder{Schedule: ScheduleDaily, AtTime: sql.NullString{String: "11:00", Valid: true}}
	next, rep, err = NextAfter(daily, now)
	if err != nil || !rep || !(next.After(now) && next.Hour() == 11 && next.Minute() == 0) {
		t.Fatalf("daily: next=%v rep=%v err=%v", next, rep, err)
	}

	once := Reminder{Schedule: ScheduleOnce}
	next, rep, err = NextAfter(once, now)
	if err != nil || rep || !next.IsZero() {
		t.Fatalf("once: next=%v rep=%v err=%v", next, rep, err)
	}
}
