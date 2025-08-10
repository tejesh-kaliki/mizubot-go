package scheduler

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"mizubot-go/internal/reminders"

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

func TestSchedulerSendsAndReschedules(t *testing.T) {
	db := openTestDB(t)
	store := reminders.NewStore(db)
	now := time.Now().Add(-time.Second)
	r := &reminders.Reminder{UserID: "u", ChannelID: "c", Message: "msg", Schedule: reminders.ScheduleHourly, NextRun: now}
	if err := store.Create(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	var sent int32
	s := New(store, func(channelID, content string) error {
		atomic.AddInt32(&sent, 1)
		return nil
	}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&sent) == 0 {
		t.Fatalf("expected at least 1 send")
	}
}
