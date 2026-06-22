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
        cron_expr TEXT NOT NULL DEFAULT '',
        once INTEGER NOT NULL DEFAULT 0,
        timezone TEXT NOT NULL DEFAULT 'UTC',
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
	r := &Reminder{UserID: "u", ChannelID: "c", Message: "hello", Schedule: ScheduleHourly, CronExpr: "0 * * * *", Timezone: "UTC", NextRun: now.Add(time.Hour)}
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

func TestDeleteReminderWithDetails(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	service := NewService(store)

	r := &Reminder{
		UserID:    "u",
		ChannelID: "c",
		Message:   "hello",
		Schedule:  ScheduleDaily,
		CronExpr:  "0 9 * * *",
		Timezone:  "UTC",
		NextRun:   time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
	}
	if err := store.Create(context.Background(), r); err != nil {
		t.Fatalf("create: %v", err)
	}

	deleted, ok, err := service.DeleteReminderWithDetails(context.Background(), r.ID, "u")
	if err != nil || !ok {
		t.Fatalf("delete with details: %v ok=%v", err, ok)
	}
	if deleted.Message != "hello" || deleted.ChannelID != "c" || deleted.CronExpr != "0 9 * * *" {
		t.Fatalf("deleted reminder mismatch: %+v", deleted)
	}

	list, err := store.ListByUser(context.Background(), "u")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected reminder to be deleted, got %+v", list)
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

	cronReminder := Reminder{Schedule: ScheduleDaily, CronExpr: "0 12 * * *", Timezone: "UTC"}
	next, rep, err = NextAfter(cronReminder, now)
	if err != nil || !rep || !(next.After(now) && next.Hour() == 12 && next.Minute() == 0) {
		t.Fatalf("cron: next=%v rep=%v err=%v", next, rep, err)
	}
}

func TestNextRunForScheduleOnceRelative(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		at   string
		want time.Time
	}{
		{at: "10m", want: now.Add(10 * time.Minute)},
		{at: "in 2h", want: now.Add(2 * time.Hour)},
		{at: "3d", want: now.Add(72 * time.Hour)},
	}

	for _, tt := range tests {
		got, atTime, err := legacyNextRunForSchedule(now, ScheduleOnce, tt.at)
		if err != nil {
			t.Fatalf("nextRunForSchedule(%q): %v", tt.at, err)
		}
		if !got.Equal(tt.want) {
			t.Fatalf("nextRunForSchedule(%q) = %v, want %v", tt.at, got, tt.want)
		}
		if !atTime.Valid || atTime.String != tt.want.Format(time.RFC3339) {
			t.Fatalf("at_time for %q = %+v, want %s", tt.at, atTime, tt.want.Format(time.RFC3339))
		}
	}
}

func TestNormalizeScheduleUsesTimezone(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got, err := normalizeSchedule(now, normalizeInput{
		Schedule: ScheduleDaily,
		At:       "09:00",
		Timezone: "Asia/Kolkata",
	})
	if err != nil {
		t.Fatalf("normalizeSchedule: %v", err)
	}
	if got.Timezone != "Asia/Kolkata" {
		t.Fatalf("timezone = %q, want Asia/Kolkata", got.Timezone)
	}
	if got.CronExpr != "0 9 * * *" {
		t.Fatalf("cron = %q, want 0 9 * * *", got.CronExpr)
	}
	want := time.Date(2026, 1, 1, 3, 30, 0, 0, time.UTC)
	if !got.NextRun.Equal(want) {
		t.Fatalf("next_run = %v, want %v", got.NextRun, want)
	}
}
